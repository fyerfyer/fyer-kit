package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mattn/go-shellwords"
	"github.com/spf13/cobra"
)

// interactiveCmd 表示交互式命令，用于启动一个REPL
var interactiveCmd = &cobra.Command{
	Use:   "interactive",
	Short: "Start an interactive session",
	Long: `Start an interactive session with the queue CLI.
Commands can be entered directly at the prompt.
Type 'exit' or 'quit' to exit, or press Ctrl+C.`,
	Aliases: []string{"i", "shell"},
	Run: func(cmd *cobra.Command, args []string) {
		runInteractiveMode()
	},
}

func init() {
	rootCmd.AddCommand(interactiveCmd)
}

func runInteractiveMode() {
	fmt.Println("Queue CLI Interactive Mode")
	fmt.Println("Type 'help' for available commands or 'exit' to quit")

	// 设置信号处理，捕获Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// 创建一个channel，用于通知主循环何时退出
	doneChan := make(chan struct{})

	// 创建一个goroutine来处理信号
	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt signal, exiting...")
		close(doneChan)
	}()

	scanner := bufio.NewScanner(os.Stdin)

	for {
		// 检查是否应该退出
		select {
		case <-doneChan:
			return
		default:
			// 继续处理输入
		}

		// 使用自定义提示符
		fmt.Print("> ")

		if !scanner.Scan() {
			break
		}

		// 读取用户输入
		input := scanner.Text()
		input = strings.TrimSpace(input)

		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			fmt.Println("Exiting...")
			return
		}

		// 解析并执行命令
		executeCommand(input)
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
	}
}

func executeCommand(input string) {
	// 使用shellwords解析命令行参数
	parser := shellwords.NewParser()
	args, err := parser.Parse(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing command: %v\n", err)
		return
	}

	if len(args) == 0 {
		return
	}

	// 存储并恢复原始os.Args
	oldArgs := os.Args
	os.Args = append([]string{"qcli"}, args...)

	// 使用根命令来查找和执行命令
	cmd := rootCmd
	cmd.SetArgs(args)

	// 如果遇到错误，捕获错误而不是退出程序
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true

	err = cmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}

	// 回复原有的os.Args
	// 这一步是必要的，因为我们在交互模式中修改了os.Args
	os.Args = oldArgs
}
