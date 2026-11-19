package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// hasPwsh 判断装了 PowerShell 7（测试里可替换）。
var hasPwsh = func() bool {
	_, err := exec.LookPath("pwsh.exe")
	return err == nil
}

// cmdLines 把多行命令接成 cmd 的一行：逐行执行（和 sh -c 多行一样，前一行失败不影响后一行）。
func cmdLines(command string) string {
	var parts []string
	for _, l := range strings.Split(strings.ReplaceAll(command, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, " & ")
}

// gitBash：Windows 上 bash / sh 用 Git for Windows 自带的。直接按名字找可能找到 System32\bash.exe，
// 那是 WSL 的 bash，脚本会跑在 Linux 里（路径、环境变量、工具链都不是 Windows 的）。找不到 Git Bash 时原样返回。
func gitBash(cmd string) string {
	if runtime.GOOS != "windows" || (cmd != "bash" && cmd != "sh") {
		return cmd
	}
	var cands []string
	if git, err := exec.LookPath("git.exe"); err == nil {
		// …\Git\cmd\git.exe 或 …\Git\mingw64\bin\git.exe
		for _, root := range []string{filepath.Dir(filepath.Dir(git)), filepath.Dir(filepath.Dir(filepath.Dir(git)))} {
			cands = append(cands, filepath.Join(root, "bin", "bash.exe"))
		}
	}
	for _, k := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
		if v := os.Getenv(k); v != "" {
			cands = append(cands, filepath.Join(v, "Git", "bin", "bash.exe"))
		}
	}
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		cands = append(cands, filepath.Join(v, "Programs", "Git", "bin", "bash.exe"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return cmd
}
