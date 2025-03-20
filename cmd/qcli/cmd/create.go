package cmd

import (
	"fmt"
	"strconv"

	"github.com/fyerfyer/fyer-kit/internal/queueservice"
	"github.com/spf13/cobra"
)

// createCmd 表示create命令，用于创建新队列
var createCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new queue",
	Long: `Create a new queue with specified options.
You can create either blocking or non-blocking queues with custom capacity.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// 获取队列名称
		name := args[0]

		// 获取参数
		queueType, _ := cmd.Flags().GetString("type")
		capacity, _ := cmd.Flags().GetInt("capacity")
		enqueueTimeout, _ := cmd.Flags().GetInt("enqueue-timeout")
		dequeueTimeout, _ := cmd.Flags().GetInt("dequeue-timeout")

		// 验证队列类型
		var qType queueservice.QueueType
		switch queueType {
		case "blocking", "b":
			qType = queueservice.BlockingQueue
		case "nonblocking", "nb":
			qType = queueservice.NonBlockingQueue
		default:
			return fmt.Errorf("invalid queue type: %s, must be 'blocking' or 'nonblocking'", queueType)
		}

		// 创建队列选项
		opts := queueservice.QueueOptions{
			Type:           qType,
			Capacity:       capacity,
			EnqueueTimeout: enqueueTimeout,
			DequeueTimeout: dequeueTimeout,
		}

		// 创建队列
		service := GetQueueService()
		if err := service.CreateQueue(name, opts); err != nil {
			return fmt.Errorf("failed to create queue: %w", err)
		}

		// 显示成功信息
		fmt.Printf("Queue '%s' created successfully.\n", name)
		fmt.Printf("Type: %s\n", qType)
		fmt.Printf("Capacity: %s\n", formatCapacity(capacity))

		if enqueueTimeout > 0 {
			fmt.Printf("Enqueue timeout: %d ms\n", enqueueTimeout)
		}

		if dequeueTimeout > 0 {
			fmt.Printf("Dequeue timeout: %d ms\n", dequeueTimeout)
		}

		return nil
	},
}

// formatCapacity 格式化容量显示
func formatCapacity(capacity int) string {
	if capacity <= 0 {
		return "unbounded"
	}
	return strconv.Itoa(capacity)
}

func init() {
	rootCmd.AddCommand(createCmd)

	// 添加参数
	createCmd.Flags().StringP("type", "t", "blocking", "Queue type: 'blocking' or 'nonblocking'")
	createCmd.Flags().IntP("capacity", "c", 0, "Queue capacity (0 for unbounded)")
	createCmd.Flags().Int("enqueue-timeout", 0, "Timeout for enqueue operations in milliseconds (0 for no timeout)")
	createCmd.Flags().Int("dequeue-timeout", 0, "Timeout for dequeue operations in milliseconds (0 for no timeout)")
}
