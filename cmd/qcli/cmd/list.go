package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/fyerfyer/fyer-kit/internal/queueservice"
	"github.com/spf13/cobra"
)

// listCmd 表示list命令，用于列出所有队列
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all queues",
	Long:  `Display a list of all available queues and their basic information.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 获取队列服务
		service := GetQueueService()

		// 获取所有队列
		queues := service.ListQueues()

		// 检查是否有队列
		if len(queues) == 0 {
			fmt.Println("No queues available.")
			return
		}

		// 使用详细模式还是表格模式
		verbose, _ := cmd.Flags().GetBool("verbose")

		if verbose {
			// 详细模式：显示每个队列的完整信息
			fmt.Printf("Found %d queue(s):\n\n", len(queues))
			for i, info := range queues {
				if i > 0 {
					fmt.Println("---")
				}
				fmt.Print(queueservice.FormatQueueInfo(info))
			}
		} else {
			// 表格模式：使用表格格式显示简洁信息
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tSIZE\tCAPACITY\tOPERATIONS")

			for _, info := range queues {
				capacityStr := fmt.Sprintf("%d", info.Stats.Capacity)
				if info.Stats.Capacity <= 0 {
					capacityStr = "unbounded"
				}

				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%d enq, %d deq\n",
					info.Name,
					info.Type,
					info.Stats.Size,
					capacityStr,
					info.Stats.Enqueued,
					info.Stats.Dequeued)
			}
			w.Flush()
		}
	},
}

func init() {
	rootCmd.AddCommand(listCmd)

	// 添加参数
	listCmd.Flags().BoolP("verbose", "v", false, "Show detailed information for each queue")
}
