package cmd

import (
	"context"
	"fmt"
	"os"

	"connectrpc.com/connect"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/spf13/cobra"

	api "github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
	"github.com/furisto/construct/frontend/cli/pkg/terminal"
)

type resumeOptions struct {
	last  bool
	all   bool
	limit int
}

func NewResumeCmd() *cobra.Command {
	options := &resumeOptions{}

	cmd := &cobra.Command{
		Use:   "resume [task-id] [flags]",
		Short: "Resume an existing conversation",
		Long: `Resume an existing conversation from where you left off. If no task ID is provided, shows an interactive picker of recent tasks. Supports partial ID matching for convenience.

The resumed conversation will restore the full context including:
- All previous messages in the conversation
- The agent that was being used
- The workspace directory settings

Examples:
  # Show interactive picker to select from recent tasks
  construct resume

  # Resume the most recent task immediately
  construct resume --last

  # Resume specific task by full ID
  construct resume 01974c1d-0be8-70e1-88b4-ad9462fff25e

  # Show more tasks, including non-interactive ones, in the picker
  construct resume --all --limit 20`,
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := getAPIClient(cmd.Context())

			return fail.HandleError(handleResumeCommand(cmd.Context(), apiClient, options, args))
		},
	}

	cmd.Flags().BoolVar(&options.last, "last", false, "Resume the most recent task immediately")
	cmd.Flags().BoolVar(&options.all, "all", false, "Show all tasks in the picker, including non-interactive ones")
	cmd.Flags().IntVar(&options.limit, "limit", 10, "Maximum number of tasks to show in the picker")

	return cmd
}

func handleResumeCommand(ctx context.Context, apiClient *api.Client, options *resumeOptions, args []string) error {
	if len(args) > 0 {
		taskID := args[0]
		return resumeTaskByID(ctx, apiClient, taskID)
	}

	if options.last {
		return resumeMostRecentTask(ctx, apiClient)
	}

	return showTaskPicker(ctx, apiClient, options)
}

func resumeTaskByID(ctx context.Context, apiClient *api.Client, taskID string) error {
	task, err := resolveTaskID(ctx, apiClient, taskID)
	if err != nil {
		return fmt.Errorf("failed to resolve task ID %s: %w", taskID, err)
	}

	return resumeTask(ctx, apiClient, task)
}

func resumeMostRecentTask(ctx context.Context, apiClient *api.Client) error {
	resp, err := apiClient.Task().ListTasks(ctx, &connect.Request[v1.ListTasksRequest]{
		Msg: &v1.ListTasksRequest{
			Filter:    &v1.ListTasksRequest_Filter{},
			SortField: api.Ptr(v1.SortField_SORT_FIELD_UPDATED_AT),
			SortOrder: api.Ptr(v1.SortOrder_SORT_ORDER_DESC),
			PageSize:  api.Ptr(int32(1)),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(resp.Msg.Tasks) == 0 {
		return fmt.Errorf("no tasks created yet")
	}

	mostRecentTask := resp.Msg.Tasks[0]
	return resumeTaskByID(ctx, apiClient, mostRecentTask.Metadata.Id)
}

func resumeTask(ctx context.Context, apiClient *api.Client, task *v1.Task) error {
	agentResp, err := apiClient.Agent().GetAgent(ctx, &connect.Request[v1.GetAgentRequest]{
		Msg: &v1.GetAgentRequest{Id: PtrToString(task.Spec.AgentId)},
	})
	if err != nil {
		return fmt.Errorf("failed to get agent: %w", err)
	}

	return startInteractiveSession(ctx, apiClient, task, agentResp.Msg.Agent)
}

func showTaskPicker(ctx context.Context, apiClient *api.Client, options *resumeOptions) error {
	resp, err := apiClient.Task().ListTasks(ctx, &connect.Request[v1.ListTasksRequest]{
		Msg: &v1.ListTasksRequest{
			PageSize:  api.Ptr(int32(options.limit)),
			SortField: api.Ptr(v1.SortField_SORT_FIELD_CREATED_AT),
			SortOrder: api.Ptr(v1.SortOrder_SORT_ORDER_DESC),
			Filter: &v1.ListTasksRequest_Filter{
				HasMessages: api.Ptr(true),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(resp.Msg.Tasks) == 0 {
		return fmt.Errorf("no tasks found")
	}

	headers := []string{"ID", "Created", "Updated", "Workspace"}
	var tableRows []terminal.TableRow

	for _, task := range resp.Msg.Tasks {
		workspace := task.Spec.Workspace
		if workspace == "" {
			workspace = "unspecified"
		}

		tableRows = append(tableRows, terminal.TableRow{
			ID:          task.Metadata.Id,
			CreatedAt:   task.Metadata.CreatedAt.AsTime(),
			UpdatedAt:   task.Metadata.UpdatedAt.AsTime(),
			Workspace:   workspace,
			Description: task.Spec.Description,
			Task:        task,
		})
	}

	table := terminal.NewSelectableTable("Select a task to resume", headers, tableRows)
	program := tea.NewProgram(table, tea.WithAltScreen())

	finalModel, err := program.Run()
	if err != nil {
		return fmt.Errorf("error running task picker: %w", err)
	}

	tableModel := finalModel.(*terminal.SelectableTable)
	if tableModel.IsCancelled() {
		return nil
	}

	selectedTask := tableModel.GetSelectedTask()
	if selectedTask == nil {
		return fmt.Errorf("no task selected")
	}

	return resumeTask(ctx, apiClient, selectedTask)
}

func resolveTaskID(ctx context.Context, apiClient *api.Client, taskID string) (*v1.Task, error) {
	if len(taskID) < 8 {
		return nil, fmt.Errorf("task ID must be at least 4 characters long")
	}

	parsedTaskID, err := uuid.Parse(taskID)
	if err == nil {
		taskResp, err := apiClient.Task().GetTask(ctx, &connect.Request[v1.GetTaskRequest]{
			Msg: &v1.GetTaskRequest{Id: parsedTaskID.String()},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get task %s: %w", taskID, err)
		}

		return taskResp.Msg.Task, nil
	}

	// Otherwise, try to find a task with a matching prefix
	resp, err := apiClient.Task().ListTasks(ctx, &connect.Request[v1.ListTasksRequest]{
		Msg: &v1.ListTasksRequest{
			Filter: &v1.ListTasksRequest_Filter{
				TaskIdPrefix: &taskID,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	if len(resp.Msg.Tasks) == 0 {
		return nil, fmt.Errorf("no task found matching %s", taskID)
	}

	if len(resp.Msg.Tasks) > 1 {
		return nil, fmt.Errorf("multiple tasks found matching %s", taskID)
	}

	return resp.Msg.Tasks[0], nil
}

func startInteractiveSession(ctx context.Context, apiClient *api.Client, task *v1.Task, agent *v1.Agent) error {
	program := tea.NewProgram(
		terminal.NewModel(ctx, apiClient, task, agent),
		tea.WithAltScreen(),
	)

	fmt.Printf("Subscribed to task %s\n", task.Metadata.Id)
	go func() {
		watch, err := apiClient.Task().Subscribe(ctx, &connect.Request[v1.SubscribeRequest]{
			Msg: &v1.SubscribeRequest{
				TaskId: task.Metadata.Id,
			},
		})
		if err != nil {
			fmt.Printf("error subscribing to task: %v\n", err)
			return
		}

		defer watch.Close()

		for watch.Receive() {
			msg := watch.Msg()
			program.Send(msg.Message)
		}

		if err := watch.Err(); err != nil {
			fmt.Printf("error watching task: %v\n", err)
		}
	}()

	tempFile, err := os.CreateTemp("", "construct-resume-*")
	if err != nil {
		return err
	}

	tea.LogToFile(tempFile.Name(), "debug")

	if _, err := program.Run(); err != nil {
		return fmt.Errorf("error running program: %w", err)
	}

	return nil
}
