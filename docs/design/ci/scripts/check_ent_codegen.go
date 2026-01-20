// scripts/ci/check_ent_codegen.go

/*
Ent 代码生成同步检查 - CI 强制执行

🛑 检查规则：
1. 运行 `go generate ./ent` 后检查 git diff
2. 如果有差异，说明 ent/ 目录代码与 ent/schema/ 不同步
3. 开发者必须提交生成的代码

使用方式：
  go run scripts/ci/check_ent_codegen.go

或在 CI 中：
  cd ent && go generate . && git diff --exit-code
*/

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	// 检查 ent 目录是否存在
	if _, err := os.Stat("ent"); os.IsNotExist(err) {
		fmt.Println("⚠️ ent/ 目录不存在，跳过检查")
		os.Exit(0)
	}

	// 检查 ent/schema 目录是否存在
	if _, err := os.Stat("ent/schema"); os.IsNotExist(err) {
		fmt.Println("⚠️ ent/schema/ 目录不存在，跳过检查")
		os.Exit(0)
	}

	fmt.Println("🔄 运行 go generate ./ent ...")

	// 运行 go generate
	generateCmd := exec.Command("go", "generate", "./ent")
	generateCmd.Stdout = os.Stdout
	generateCmd.Stderr = os.Stderr
	if err := generateCmd.Run(); err != nil {
		fmt.Printf("❌ go generate 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("🔍 检查 ent/ 目录是否有未提交的变更...")

	// 检查 git diff
	diffCmd := exec.Command("git", "diff", "--name-only", "ent/")
	var diffOutput bytes.Buffer
	diffCmd.Stdout = &diffOutput
	diffCmd.Stderr = os.Stderr
	if err := diffCmd.Run(); err != nil {
		fmt.Printf("❌ git diff 失败: %v\n", err)
		os.Exit(1)
	}

	// 检查是否有差异
	changedFiles := strings.TrimSpace(diffOutput.String())
	if changedFiles != "" {
		fmt.Println("❌ Ent 生成代码不同步!")
		fmt.Println("\n以下文件需要重新生成并提交:")
		for _, file := range strings.Split(changedFiles, "\n") {
			if file != "" {
				fmt.Printf("  - %s\n", file)
			}
		}
		fmt.Println("\n📋 修复方法:")
		fmt.Println("  1. 运行: go generate ./ent")
		fmt.Println("  2. 提交生成的文件: git add ent/ && git commit")
		os.Exit(1)
	}

	// 检查是否有未跟踪的新文件
	statusCmd := exec.Command("git", "status", "--porcelain", "ent/")
	var statusOutput bytes.Buffer
	statusCmd.Stdout = &statusOutput
	if err := statusCmd.Run(); err != nil {
		fmt.Printf("❌ git status 失败: %v\n", err)
		os.Exit(1)
	}

	untrackedFiles := strings.TrimSpace(statusOutput.String())
	if untrackedFiles != "" {
		hasUntracked := false
		for _, line := range strings.Split(untrackedFiles, "\n") {
			if strings.HasPrefix(line, "??") {
				hasUntracked = true
				break
			}
		}
		if hasUntracked {
			fmt.Println("❌ ent/ 目录有未跟踪的新文件!")
			fmt.Println("\n请添加并提交这些文件:")
			fmt.Println(untrackedFiles)
			os.Exit(1)
		}
	}

	fmt.Println("✅ Ent 代码生成同步检查通过")
}
