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
	agent string
}

func NewResumeCmd() *cobra.Command {
	options := &resumeOptions{}

	cmd := &cobra.Command{
		Use:   "resume [task-id] [flags]",
		Short: "Continue a previous task",
		Long: `Continue a previous task.

Pick up a conversation where you left off. 'construct resume' restores the full 
context of a previous task, including the agent and all messages.

If no task-id is provided, an interactive menu will display recent tasks to 
choose from. Partial ID matching is supported.`,
		Example: `  # Show an interactive picker to select a recent task
  construct resume

  # Resume the most recent task immediately
  construct resume --last

  # Resume a specific task by its ID
  construct resume 01974c1d-0be8-70e1-88b4-ad9462fff25e`,
		GroupID: "core",
		RunE: func(cmd *cobra.Command, args []string) error {
			apiClient := getAPIClient(cmd.Context())

			return fail.HandleError(cmd, handleResumeCommand(cmd.Context(), apiClient, options, args))
		},
	}

	cmd.Flags().BoolVar(&options.last, "last", false, "Immediately resume the most recent session without showing the interactive picker")
	cmd.Flags().BoolVar(&options.all, "all", false, "Show all tasks in the picker, including non-interactive ones")
	cmd.Flags().IntVar(&options.limit, "limit", 10, "Maximum number of tasks to show in the picker")
	cmd.Flags().StringVar(&options.agent, "agent", "", "Resume the task with a specific agent")

	return cmd
}

func handleResumeCommand(ctx context.Context, apiClient *api.Client, options *resumeOptions, args []string) error {
	if len(args) > 0 {
		taskID := args[0]
		return resumeTaskByID(ctx, apiClient, taskID, options.agent)
	}

	if options.last {
		return resumeMostRecentTask(ctx, apiClient, options.agent)
	}

	return showTaskPicker(ctx, apiClient, options)
}

func resumeTaskByID(ctx context.Context, apiClient *api.Client, taskID string, agent string) error {
	task, err := resolveTaskID(ctx, apiClient, taskID)
	if err != nil {
		return fmt.Errorf("failed to resolve task ID %s: %w", taskID, err)
	}

	return resumeTask(ctx, apiClient, task, agent)
}

func resumeMostRecentTask(ctx context.Context, apiClient *api.Client, agent string) error {
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
	return resumeTaskByID(ctx, apiClient, mostRecentTask.Metadata.Id, agent)
}

func resumeTask(ctx context.Context, apiClient *api.Client, task *v1.Task, agent string) error {
	var agentID string
	if agent != "" {
		var err error
		agentID, err = getAgentID(ctx, apiClient, agent)
		if err != nil {
			return fmt.Errorf("failed to get agent %s: %w", agent, err)
		}
	} else {
		agentID = *task.Spec.AgentId
	}

	agentResp, err := apiClient.Agent().GetAgent(ctx, &connect.Request[v1.GetAgentRequest]{
		Msg: &v1.GetAgentRequest{Id: agentID},
	})
	if err != nil {
		return fmt.Errorf("failed to get agent %s: %w", agentID, err)
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
			ID:           task.Metadata.Id,
			CreatedAt:    task.Metadata.CreatedAt.AsTime(),
			UpdatedAt:    task.Metadata.UpdatedAt.AsTime(),
			Workspace:    workspace,
			MessageCount: task.Status.MessageCount,
			Description:  task.Spec.Description,
			Task:         task,
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

	return resumeTask(ctx, apiClient, selectedTask, options.agent)
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
		terminal.NewSession(ctx, apiClient, task, agent),
		tea.WithAltScreen(),
	)

	fmt.Printf("Subscribed to task %s\n", task.Metadata.Id)
	go func() {
		taskID := task.Metadata.Id
		watch, err := apiClient.Event().Subscribe(ctx, &connect.Request[v1.EventSubscribeRequest]{
			Msg: &v1.EventSubscribeRequest{
				EventTypes: []string{"task.*", "message.*"},
				TaskId:     &taskID,
			},
		})
		if err != nil {
			fmt.Printf("error subscribing to task: %v\n", err)
			return
		}

		defer watch.Close()

		for watch.Receive() {
			msg := watch.Msg()
			if msg.Event == nil {
				continue
			}
			switch payload := msg.Event.Payload.(type) {
			case *v1.Event_Message:
				if payload.Message != nil && payload.Message.Message != nil {
					program.Send(payload.Message.Message)
				}
			case *v1.Event_MessageChunk:
				if payload.MessageChunk != nil {
					program.Send(payload.MessageChunk)
				}
			case *v1.Event_Task:
				if payload.Task != nil {
					program.Send(payload.Task)
				}
			}
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
