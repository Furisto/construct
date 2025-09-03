package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"connectrpc.com/connect"
	"github.com/furisto/construct/api/go/client"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/frontend/cli/pkg/fail"
	"github.com/furisto/construct/shared/conv"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type execOutputFormat string

const (
	execOutputFormatText execOutputFormat = "text"
	execOutputFormatJSON execOutputFormat = "json"
	execOutputFormatYAML execOutputFormat = "yaml"
)

func (e *execOutputFormat) String() string {
	if e == nil || *e == "" {
		return string(execOutputFormatText)
	}
	return string(*e)
}

func (e *execOutputFormat) Set(v string) error {
	switch v {
	case "text", "json", "yaml":
		*e = execOutputFormat(v)
		return nil
	default:
		return errors.New(`must be one of "text", "json", or "yaml"`)
	}
}

func (e *execOutputFormat) Type() string {
	return "format"
}

type execOptions struct {
	Agent     string
	Workspace string
	MaxTurns  int
	Continue  string
	Files     []string
	Format    execOutputFormat
}

func NewExecCmd() *cobra.Command {
	options := execOptions{
		Format: execOutputFormatText,
	}

	cmd := &cobra.Command{
		Use:     "exec [question]",
		Short:   "Exec a question to the AI",
		Args:    cobra.MaximumNArgs(1),
		GroupID: "core",
		Example: `  # Simple question
  construct exec "What is 2+2?"

  # Use a specific agent
  construct exec "Review this code for security issues" --agent security-reviewer

  # Include files as context
  construct exec "What does this code do?" --file main.go --file utils.go

  # Pipe input with question and file context
  cat main.go | construct exec "What does this code do?" --file config.yaml

  # Give agent more turns for complex tasks
  construct exec "Debug why the build is failing" --max-turns 10

  # Get JSON output for scripting
  construct ask "List all Go files" --output json
  
  # Continue the previous task
  construct exec "Continue refactoring the code" --continue`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var question string
			if len(args) > 0 {
				question = args[0]
			}
			return fail.HandleError(handleExec(cmd.Context(), cmd, options, question))
		},
	}

	setupFlags(cmd, &options)
	return cmd
}

func setupFlags(cmd *cobra.Command, options *execOptions) {
	cmd.Flags().StringVarP(&options.Agent, "agent", "a", "", "The agent to use (name or ID)")
	cmd.Flags().StringVarP(&options.Workspace, "workspace", "w", "", "The workspace directory")
	cmd.Flags().IntVar(&options.MaxTurns, "max-turns", 5, "Maximum number of turns for the conversation")
	cmd.Flags().StringSliceVarP(&options.Files, "file", "f", []string{}, "Files to include as context (can be used multiple times)")
	cmd.Flags().StringVarP(&options.Continue, "continue", "c", "", "Continue the previous task")
	cmd.Flags().VarP(&options.Format, "output", "o", "The format to output the result in")
	cmd.Flags().Lookup("continue").NoOptDefVal = "last"
}

func handleExec(ctx context.Context, cmd *cobra.Command, options execOptions, question string) error {
	client := getAPIClient(ctx)

	question, err := prepareQuestion(question, options.Files, cmd.InOrStdin(), getFileSystem(ctx))
	if err != nil {
		return err
	}

	task, err := setupTask(ctx, cmd, client, options)
	if err != nil {
		return err
	}

	if err := sendMessage(ctx, client, task.Metadata.Id, question); err != nil {
		return err
	}

	return handleResponseStream(ctx, cmd, client, task.Metadata.Id, options.Format)
}

func prepareQuestion(question string, files []string, stdin io.Reader, fs afero.Fs) (string, error) {
	question, err := getQuestion(question, stdin)
	if err != nil {
		return "", err
	}

	return buildMessage(question, files, fs)
}

func getQuestion(question string, stdin io.Reader) (string, error) {
	var stdinContent string

	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		content, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read input from stdin: %w", err)
		}
		stdinContent = strings.TrimSpace(string(content))
	}

	switch {
	case stdinContent != "" && question != "":
		return fmt.Sprintf("%s\n\n%s", question, stdinContent), nil
	case stdinContent != "":
		return stdinContent, nil
	case question != "":
		return question, nil
	}

	return "", fmt.Errorf("no question provided - provide as argument or pipe via stdin")
}

func buildMessage(question string, files []string, fs afero.Fs) (string, error) {
	if len(files) == 0 {
		return question, nil
	}

	var builder strings.Builder
	builder.WriteString(question + "\n")
	builder.WriteString("--- File Context ---\n")

	for i, filepath := range files {
		content, err := fs.Open(filepath)
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", filepath, err)
		}

		builder.WriteString(fmt.Sprintf("### %s\n```\n", filepath))
		_, err = io.Copy(&builder, content)
		content.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", filepath, err)
		}
		if i < len(files)-1 {
			builder.WriteString("\n```\n\n")
		} else {
			builder.WriteString("\n```")
		}
	}

	return builder.String(), nil
}

func setupTask(ctx context.Context, cmd *cobra.Command, client *client.Client, options execOptions) (task *v1.Task, err error) {
	workspace := options.Workspace
	if workspace == "" {
		workspace, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	agentID, err := getAgentID(ctx, client, options.Agent)
	if err != nil {
		return nil, err
	}

	if cmd.Flags().Changed("continue") {
		return continueTask(ctx, options, client)
	}

	return createTask(ctx, client, agentID, workspace)
}

func continueTask(ctx context.Context, options execOptions, client *client.Client) (*v1.Task, error) {
	if options.Continue == "last" {
		tasks, err := client.Task().ListTasks(ctx, &connect.Request[v1.ListTasksRequest]{
			Msg: &v1.ListTasksRequest{
				SortField: conv.Ptr(v1.SortField_SORT_FIELD_UPDATED_AT),
				SortOrder: conv.Ptr(v1.SortOrder_SORT_ORDER_DESC),
			},
		})

		if err != nil {
			return nil, fmt.Errorf("failed to list tasks: %w", err)
		}

		if len(tasks.Msg.Tasks) > 0 {
			return tasks.Msg.Tasks[0], nil
		} else {
			return nil, fmt.Errorf("no tasks found")
		}
	} else {
		resp, err := client.Task().GetTask(ctx, &connect.Request[v1.GetTaskRequest]{
			Msg: &v1.GetTaskRequest{
				Id: options.Continue,
			},
		})

		if err != nil {
			return nil, fmt.Errorf("failed to get task: %w", err)
		}
		return resp.Msg.Task, nil
	}
}

func createTask(ctx context.Context, client *client.Client, agentID, workspace string) (*v1.Task, error) {
	taskResp, err := client.Task().CreateTask(ctx, &connect.Request[v1.CreateTaskRequest]{
		Msg: &v1.CreateTaskRequest{
			AgentId:          agentID,
			ProjectDirectory: workspace,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}
	return taskResp.Msg.Task, nil
}

func sendMessage(ctx context.Context, client *client.Client, taskID, message string) error {
	_, err := client.Message().CreateMessage(ctx, &connect.Request[v1.CreateMessageRequest]{
		Msg: &v1.CreateMessageRequest{
			TaskId: taskID,
			Content: []*v1.MessagePart{
				{
					Data: &v1.MessagePart_Text_{
						Text: &v1.MessagePart_Text{
							Content: message,
						},
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return nil
}

func handleResponseStream(ctx context.Context, cmd *cobra.Command, client *client.Client, taskID string, format execOutputFormat) error {
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	stream, err := client.Task().Subscribe(streamCtx, &connect.Request[v1.SubscribeRequest]{
		Msg: &v1.SubscribeRequest{
			TaskId: taskID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to task: %w", err)
	}

	for stream.Receive() {
		message := stream.Msg().GetMessage()
		if message == nil {
			continue
		}

		task, err := client.Task().GetTask(ctx, &connect.Request[v1.GetTaskRequest]{
			Msg: &v1.GetTaskRequest{
				Id: taskID,
			},
		})
		if err != nil {
			return fmt.Errorf("failed to get task: %w", err)
		}
		if err := formatMessage(task.Msg.Task, message, format, cmd); err != nil {
			return err
		}

		if message.Status != nil && message.Status.IsFinalResponse {
			streamCancel()
			break
		}
	}

	if err := stream.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

func formatMessage(task *v1.Task, message *v1.Message, format execOutputFormat, cmd *cobra.Command) error {
	switch format {
	case execOutputFormatText:
		return formatTextMessage(message, cmd)
	case execOutputFormatJSON:
		jsonBytes, err := formatJSONMessage(task, message)
		if err != nil {
			return err
		}
		cmd.Println(string(jsonBytes))
	case execOutputFormatYAML:
		yamlBytes, err := formatYAMLMessage(task, message)
		if err != nil {
			return err
		}
		cmd.Println(string(yamlBytes))
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	return nil
}

func formatTextMessage(message *v1.Message, cmd *cobra.Command) error {
	for _, part := range message.Spec.Content {
		switch partData := part.Data.(type) {
		case *v1.MessagePart_Text_:
			cmd.Println(partData.Text.Content)
		}
	}
	return nil
}

func formatJSONMessage(task *v1.Task, message *v1.Message) ([]byte, error) {
	answer := ConvertToDisplayAnswer(task, message)

	jsonBytes, err := json.Marshal(answer)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	return jsonBytes, nil
}

func formatYAMLMessage(task *v1.Task, message *v1.Message) ([]byte, error) {
	answer := ConvertToDisplayAnswer(task, message)

	yamlBytes, err := yaml.Marshal(answer)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal to YAML: %w", err)
	}

	return yamlBytes, nil
}

// func toJSON(message *v1.Message) ([]byte, error) {
// 	marshaler := protojson.MarshalOptions{
// 		Multiline:       true,
// 		Indent:          "  ",
// 		UseProtoNames:   false,
// 		EmitUnpopulated: false,
// 	}

// 	return marshaler.Marshal(message)
// }

type DisplayAnswer struct {
	TaskID string           `json:"task_id" yaml:"task_id"`
	Agent  string           `json:"agent" yaml:"agent"`
	Model  string           `json:"model" yaml:"model"`
	Turn   int64            `json:"turn" yaml:"turn"`
	Result string           `json:"result" yaml:"result"`
	Usage  DisplayTaskUsage `json:"usage" yaml:"usage"`
}

func ConvertToDisplayAnswer(task *v1.Task, message *v1.Message) *DisplayAnswer {
	answer := DisplayAnswer{
		TaskID: task.Metadata.Id,
		Agent:  *task.Spec.AgentId,
		Turn:   task.Status.Turn,
		Usage:  ConvertTaskUsageToDisplay(task.Status.Usage),
	}

	for _, part := range message.Spec.Content {
		switch partData := part.Data.(type) {
		case *v1.MessagePart_Text_:
			answer.Result = partData.Text.Content
		}
	}

	return &answer
}
