package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

// dequeueCmd 表示dequeue命令，用于从队列获取项目
var dequeueCmd = &cobra.Command{
	Use:   "dequeue [queue-name]",
	Short: "Remove and display items from a queue",
	Long: `Remove and display one or more items from a specified queue.
You can dequeue a single item or multiple items at once.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// 获取队列名称
		queueName := args[0]

		// 获取参数
		count, _ := cmd.Flags().GetInt("count")
		silent, _ := cmd.Flags().GetBool("silent")

		// 验证参数
		if count < 0 {
			return fmt.Errorf("count must be a non-negative number")
		}

		// 如果count为0，则设置为1（默认行为）
		if count == 0 {
			count = 1
		}

		service := GetQueueService()

		// 批量出队
		var dequeued int
		for i := 0; i < count; i++ {
			item, err := service.DequeueItem(queueName)
			if err != nil {
				// 如果是第一个项目就失败，返回错误
				if i == 0 {
					return fmt.Errorf("failed to dequeue item: %w", err)
				}
				// 否则中断并报告部分成功
				fmt.Printf("Dequeued %d item(s) before encountering an error: %v\n", i, err)
				break
			}

			dequeued++
			if !silent {
				fmt.Printf("Item %d: %s\n", i+1, item)
			}
		}

		// 显示汇总信息
		if silent || dequeued > 1 {
			fmt.Printf("Successfully dequeued %d item(s) from queue '%s'\n", dequeued, queueName)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(dequeueCmd)

	// 添加参数
	dequeueCmd.Flags().IntP("count", "c", 1, "Number of items to dequeue")
	dequeueCmd.Flags().BoolP("silent", "s", false, "Silent mode (don't print items)")
}
