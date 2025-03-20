package cmd

import (
	"github.com/fyerfyer/fyer-kit/internal/queueservice"
	"github.com/spf13/cobra"
)

var (
	// 队列服务实例，所有命令共享
	queueSvc queueservice.Service
)

// rootCmd 表示CLI工具的根命令
var rootCmd = &cobra.Command{
	Use:   "qcli",
	Short: "A CLI tool for managing queues",
	Long: `Queue CLI (qcli) is a command line interface for creating and managing queues.
It supports both blocking and non-blocking queues, allowing you to enqueue and dequeue items,
monitor queue statistics, and more.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 如果没有子命令被调用，显示帮助信息
		cmd.Help()
	},
}

// Execute 运行根命令并处理任何错误
func Execute() {
	// 设置队列服务实例
	queueSvc = queueservice.NewInMemoryService()

	// 直接运行交互模式
	runInteractiveMode()

	// 在程序结束时关闭队列服务
	queueSvc.Close()
}

// init 初始化根命令
func init() {
	cobra.OnInitialize(initConfig)

	// 这里可以设置持久性标志(如果需要)
	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
}

// initConfig 读取配置文件(如果有)
func initConfig() {
	// 这里可以实现配置文件读取逻辑
	// 简单起见，我们暂时不添加这些功能
}

// GetQueueService 返回队列服务实例，供子命令使用
func GetQueueService() queueservice.Service {
	return queueSvc
}
