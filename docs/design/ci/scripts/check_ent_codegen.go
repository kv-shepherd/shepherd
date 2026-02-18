//go:build ignore

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
	"sort"
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

	// 记录 go generate 前的工作区状态，避免本地已有改动导致误报。
	beforeTracked, err := gitNameOnlyDiff("ent/")
	if err != nil {
		fmt.Printf("❌ 读取 go generate 前 tracked 状态失败: %v\n", err)
		os.Exit(1)
	}
	beforeUntracked, err := gitUntracked("ent/")
	if err != nil {
		fmt.Printf("❌ 读取 go generate 前 untracked 状态失败: %v\n", err)
		os.Exit(1)
	}

	// 运行 go generate
	generateCmd := exec.Command("go", "generate", "./ent")
	generateCmd.Stdout = os.Stdout
	generateCmd.Stderr = os.Stderr
	if err := generateCmd.Run(); err != nil {
		fmt.Printf("❌ go generate 失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("🔍 检查 ent/ 目录是否有未提交的变更...")

	afterTracked, err := gitNameOnlyDiff("ent/")
	if err != nil {
		fmt.Printf("❌ 读取 go generate 后 tracked 状态失败: %v\n", err)
		os.Exit(1)
	}
	afterUntracked, err := gitUntracked("ent/")
	if err != nil {
		fmt.Printf("❌ 读取 go generate 后 untracked 状态失败: %v\n", err)
		os.Exit(1)
	}

	newTracked := diffSet(afterTracked, beforeTracked)
	newUntracked := diffSet(afterUntracked, beforeUntracked)

	if len(newTracked) > 0 {
		fmt.Println("❌ Ent 生成代码不同步!")
		fmt.Println("\n以下文件需要重新生成并提交:")
		sort.Strings(newTracked)
		for _, file := range newTracked {
			fmt.Printf("  - %s\n", file)
		}
		fmt.Println("\n📋 修复方法:")
		fmt.Println("  1. 运行: go generate ./ent")
		fmt.Println("  2. 提交生成的文件: git add ent/ && git commit")
		os.Exit(1)
	}

	if len(newUntracked) > 0 {
		sort.Strings(newUntracked)
		fmt.Println("❌ ent/ 目录有未跟踪的新文件!")
		fmt.Println("\n请添加并提交这些文件:")
		for _, file := range newUntracked {
			fmt.Printf("  - %s\n", file)
		}
		os.Exit(1)
	}

	fmt.Println("✅ Ent 代码生成同步检查通过")
}

func gitNameOnlyDiff(path string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return splitLines(out.String()), nil
}

func gitUntracked(path string) ([]string, error) {
	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard", path)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return splitLines(out.String()), nil
}

func splitLines(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func diffSet(after, before []string) []string {
	if len(after) == 0 {
		return nil
	}
	beforeSet := make(map[string]struct{}, len(before))
	for _, item := range before {
		beforeSet[item] = struct{}{}
	}
	out := make([]string, 0, len(after))
	for _, item := range after {
		if _, ok := beforeSet[item]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}
