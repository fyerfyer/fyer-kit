package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// monitorCmd 表示monitor命令，用于实时监控队列状态
var monitorCmd = &cobra.Command{
	Use:   "monitor [queue-name]",
	Short: "Monitor queue activity in real-time",
	Long: `Watch queue statistics update in real-time.
Press Ctrl+C to stop monitoring.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// 获取队列名称
		queueName := args[0]

		// 获取刷新间隔
		interval, _ := cmd.Flags().GetInt("interval")
		refreshDuration := time.Duration(interval) * time.Millisecond

		// 获取队列服务
		service := GetQueueService()

		// 检查队列是否存在
		_, err := service.GetQueue(queueName)
		if err != nil {
			return fmt.Errorf("queue '%s' not found", queueName)
		}

		// 设置信号处理，捕获Ctrl+C
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		// 显示监控启动信息
		fmt.Printf("Monitoring queue '%s' (refresh: %v, press Ctrl+C to stop)...\n\n",
			queueName, refreshDuration)

		// 记录前一次的统计信息，用于计算变化率
		var prevStats struct {
			Enqueued uint64
			Dequeued uint64
			Time     time.Time
		}
		prevStats.Time = time.Now()

		// 监控循环
		ticker := time.NewTicker(refreshDuration)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 获取最新统计信息
				stats, err := service.QueueStats(queueName)
				if err != nil {
					return fmt.Errorf("failed to get queue statistics: %w", err)
				}

				// 计算每秒操作率
				now := time.Now()
				elapsed := now.Sub(prevStats.Time).Seconds()
				enqueueRate := float64(stats.Enqueued-prevStats.Enqueued) / elapsed
				dequeueRate := float64(stats.Dequeued-prevStats.Dequeued) / elapsed

				// 清除上一次输出
				fmt.Print("\033[H\033[2J") // 清屏，移动光标到左上角

				// 显示当前时间
				fmt.Printf("Time: %s\n\n", now.Format("15:04:05"))

				// 显示基本信息
				fmt.Printf("Queue: %s\n", queueName)
				fmt.Printf("Size: %d", stats.Size)
				if stats.Capacity > 0 {
					fmt.Printf("/%d (%.1f%% full)",
						stats.Capacity, stats.Utilization()*100)
				}
				fmt.Println()

				// 显示操作统计
				fmt.Printf("Operations: %d enqueued, %d dequeued\n",
					stats.Enqueued, stats.Dequeued)
				fmt.Printf("Rate: %.2f enq/s, %.2f deq/s\n",
					enqueueRate, dequeueRate)

				// 显示其他统计信息
				if stats.EnqueueBlocks > 0 || stats.DequeueBlocks > 0 {
					fmt.Printf("Blocks: %d enqueue, %d dequeue\n",
						stats.EnqueueBlocks, stats.DequeueBlocks)
				}

				if stats.EnqueueTimeouts > 0 || stats.DequeueTimeouts > 0 {
					fmt.Printf("Timeouts: %d enqueue, %d dequeue\n",
						stats.EnqueueTimeouts, stats.DequeueTimeouts)
				}

				if stats.Rejected > 0 {
					fmt.Printf("Rejected: %d\n", stats.Rejected)
				}

				// 更新前一次统计信息
				prevStats.Enqueued = stats.Enqueued
				prevStats.Dequeued = stats.Dequeued
				prevStats.Time = now

			case <-sigChan:
				// 收到中断信号，退出监控
				fmt.Println("\nMonitoring stopped.")
				return nil
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd)

	// 添加参数
	monitorCmd.Flags().IntP("interval", "i", 1000, "Refresh interval in milliseconds")
}
