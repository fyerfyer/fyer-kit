package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fyerfyer/fyer-kit/internal/queueservice"
	"github.com/spf13/cobra"
)

// enqueueCmd 表示enqueue命令，用于向队列添加项目
var enqueueCmd = &cobra.Command{
	Use:   "enqueue [queue-name]",
	Short: "Add items to a queue",
	Long: `Add one or more items to a specified queue.
You can add a single item or read multiple items from a file.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// 获取队列名称
		queueName := args[0]

		// 获取参数
		item, _ := cmd.Flags().GetString("item")
		filePath, _ := cmd.Flags().GetString("file")

		// 检查是否同时指定了item和file
		if item != "" && filePath != "" {
			return fmt.Errorf("cannot specify both --item and --file flags at the same time")
		}

		// 检查是否没有指定item和file
		if item == "" && filePath == "" {
			return fmt.Errorf("must specify either --item or --file flag")
		}

		service := GetQueueService()

		// 从文件批量入队
		if filePath != "" {
			return enqueueFromFile(service, queueName, filePath)
		}

		// 单个项目入队
		if err := service.EnqueueItem(queueName, item); err != nil {
			return fmt.Errorf("failed to enqueue item: %w", err)
		}

		fmt.Printf("Successfully enqueued item to queue '%s'\n", queueName)
		return nil
	},
}

// enqueueFromFile 从文件中读取项目并入队
func enqueueFromFile(service queueservice.Service, queueName, filePath string) error {
	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 读取文件内容
	scanner := bufio.NewScanner(file)
	var enqueued, failed int

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // 跳过空行
		}

		if err := service.EnqueueItem(queueName, line); err != nil {
			failed++
			fmt.Printf("Failed to enqueue: %s - %v\n", line, err)
		} else {
			enqueued++
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	fmt.Printf("Bulk enqueue to queue '%s' completed: %d items enqueued, %d failed\n",
		queueName, enqueued, failed)
	return nil
}

func init() {
	rootCmd.AddCommand(enqueueCmd)

	// 添加参数
	enqueueCmd.Flags().StringP("item", "i", "", "Item to enqueue")
	enqueueCmd.Flags().StringP("file", "f", "", "File containing items to enqueue (one per line)")
}
