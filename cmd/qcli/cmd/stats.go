package cmd

import (
	"fmt"

	"github.com/fyerfyer/fyer-kit/internal/queueservice"
	"github.com/spf13/cobra"
)

// statsCmd 表示stats命令，用于显示队列的统计信息
var statsCmd = &cobra.Command{
	Use:   "stats [queue-name]",
	Short: "Display queue statistics",
	Long: `Display detailed statistics for a specified queue.
This includes size, capacity, operations count, and more.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// 获取队列名称
		queueName := args[0]

		// 获取队列服务
		service := GetQueueService()

		// 获取统计信息
		stats, err := service.QueueStats(queueName)
		if err != nil {
			return fmt.Errorf("failed to get queue statistics: %w", err)
		}

		// 格式化并显示统计信息
		fmt.Printf("Statistics for queue '%s':\n\n", queueName)
		fmt.Print(queueservice.FormatQueueStats(stats))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
