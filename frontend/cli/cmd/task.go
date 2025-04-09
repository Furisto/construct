package cmd

import (
	"time"

	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage tasks",
}

func init() {
	rootCmd.AddCommand(taskCmd)
}

type DisplayTask struct {
	Id        string           `json:"id" yaml:"id"`
	AgentId   string           `json:"agent_id" yaml:"agent_id"`
	CreatedAt time.Time        `json:"created_at" yaml:"created_at"`
	UpdatedAt time.Time        `json:"updated_at" yaml:"updated_at"`
	Usage     DisplayTaskUsage `json:"usage" yaml:"usage"`
}

type DisplayTaskUsage struct {
	InputTokens      int64   `json:"input_tokens" yaml:"input_tokens"`
	OutputTokens     int64   `json:"output_tokens" yaml:"output_tokens"`
	CacheWriteTokens int64   `json:"cache_write_tokens" yaml:"cache_write_tokens"`
	CacheReadTokens  int64   `json:"cache_read_tokens" yaml:"cache_read_tokens"`
	Cost             float64 `json:"cost" yaml:"cost"`
}

func ConvertTaskToDisplay(task *v1.Task) *DisplayTask {
	return &DisplayTask{
		Id:        task.Id,
		AgentId:   PtrToString(task.Spec.AgentId),
		Usage:     ConvertTaskUsageToDisplay(task.Status.Usage),
		CreatedAt: task.Metadata.CreatedAt.AsTime(),
		UpdatedAt: task.Metadata.UpdatedAt.AsTime(),
	}
}

func ConvertTaskUsageToDisplay(usage *v1.TaskUsage) DisplayTaskUsage {
	return DisplayTaskUsage{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheWriteTokens: usage.CacheWriteTokens,
		CacheReadTokens:  usage.CacheReadTokens,
		Cost:             usage.Cost,
	}
}
