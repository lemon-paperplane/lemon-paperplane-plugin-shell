// Package shell 是「脚本与可执行文件」插件：把任意脚本、可执行文件或一段自定义命令作为运行配置。
//
// 两种运行配置都标了 anyModule：任何模块（包括没识别出技术的工程根目录）都能建，插件不参与识别分析，
// 平时不会被拉起，只在运行这类配置时才启动。
package shell

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	sdk "github.com/lemon-paperplane/lemon-paperplane-main/lemon-paperplane-main-plugin-sdk"
)

// 运行配置类型与选项
const (
	TypeScript  = "script"
	TypeCommand = "command"

	OptFile        = "file"
	OptArgs        = "args"
	OptInterpreter = "interpreter"
	OptCwd         = "cwd"
	OptCommand     = "command"
	OptShell       = "shell"
)

type Plugin struct{}

// New 返回插件实现。
func New() any { return &Plugin{} }

// PrepareRun 组装要由内核托管的进程。
func (p *Plugin) PrepareRun(ctx *sdk.Context, in *sdk.PrepareRunParams) (*sdk.LaunchSpec, error) {
	root := in.Project.Root
	opts := in.Config.Options
	switch in.Config.Type {
	case TypeCommand:
		command := strings.TrimSpace(sdk.OptString(opts, OptCommand, ""))
		if command == "" {
			return nil, sdk.UserError("%s", ctx.T("err.noCommand"))
		}
		cwd, err := resolveCwd(ctx, root, sdk.OptString(opts, OptCwd, ""), root)
		if err != nil {
			return nil, err
		}
		cmd, args := shellCommand(sdk.OptString(opts, OptShell, "auto"), command, runtime.GOOS, os.Getenv("SHELL"))
		cmd = gitBash(cmd)
		return &sdk.LaunchSpec{Command: cmd, Args: args, Cwd: cwd, Env: in.Env, StopSignal: "SIGINT", StopTimeoutMs: 5000, Terminal: true,
			Display: commandDisplay(ctx, command)}, nil
	default: // script
		file := strings.TrimSpace(sdk.OptString(opts, OptFile, ""))
		if file == "" {
			return nil, sdk.UserError("%s", ctx.T("err.noFile"))
		}
		abs := absPath(root, file)
		if fi, err := os.Stat(abs); err != nil || fi.IsDir() {
			return nil, sdk.UserError("%s", ctx.T("err.fileMissing", file))
		}
		cmd, args, ok := resolveInterpreter(abs, sdk.OptString(opts, OptInterpreter, "auto"), root, runtime.GOOS)
		if !ok {
			return nil, sdk.UserError("%s", ctx.T("err.noInterpreter", filepath.Base(abs)))
		}
		shown := cmd // 显示用解释器名字，不显示 Git Bash 的完整路径
		cmd = gitBash(cmd)
		args = append(args, sdk.SplitArgs(sdk.OptString(opts, OptArgs, ""))...)
		cwd, err := resolveCwd(ctx, root, sdk.OptString(opts, OptCwd, ""), filepath.Dir(abs))
		if err != nil {
			return nil, err
		}
		return &sdk.LaunchSpec{Command: cmd, Args: args, Cwd: cwd, Env: in.Env, StopSignal: "SIGINT", StopTimeoutMs: 5000, Terminal: true,
			Display: displayLine(shown, args, root)}, nil
	}
}

// ApplyFix 处理「加上执行权限」。
func (p *Plugin) ApplyFix(ctx *sdk.Context, f *sdk.ApplyFixParams) (*sdk.ApplyFixResult, error) {
	if f.Fix.ID != "chmod" || f.Config == nil {
		return nil, sdk.ErrNotSupported
	}
	file := sdk.OptString(f.Config.Options, OptFile, "")
	if file == "" {
		return nil, sdk.UserError("%s", ctx.T("err.noFile"))
	}
	abs := absPath(f.Project.Root, file)
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, sdk.UserError("%s", ctx.T("err.fileMissing", file))
	}
	// 有读权限的那一级（用户/组/其他）都加上执行权限，和 chmod +x 的常见效果一致
	mode := fi.Mode().Perm()
	if err := os.Chmod(abs, mode|((mode&0o444)>>2)); err != nil {
		return nil, err
	}
	return &sdk.ApplyFixResult{Rerun: true, Message: ctx.T("msg.chmodDone", filepath.Base(abs))}, nil
}

// ---------------------------------------------------------------------------
// 解释器

// 按扩展名选解释器（自动模式下，没有 #! 时用）
var byExt = map[string][]string{
	".sh": {"bash"}, ".bash": {"bash"}, ".command": {"bash"}, ".zsh": {"zsh"}, ".fish": {"fish"},
	".js": {"node"}, ".mjs": {"node"}, ".cjs": {"node"}, ".ts": {"node"}, ".mts": {"node"},
	".rb": {"ruby"}, ".pl": {"perl"}, ".php": {"php"}, ".lua": {"lua"}, ".r": {"Rscript"},
	".swift": {"swift"}, ".go": {"go", "run"}, ".jar": {"java", "-jar"}, ".ps1": {"pwsh", "-NoProfile", "-File"},
	".applescript": {"osascript"}, ".scpt": {"osascript"}, ".groovy": {"groovy"}, ".kts": {"kotlin"},
}

// 显式选的解释器
var named = map[string][]string{
	"bash": {"bash"}, "zsh": {"zsh"}, "sh": {"sh"}, "node": {"node"}, "ruby": {"ruby"},
	"perl": {"perl"}, "php": {"php"}, "pwsh": {"pwsh", "-NoProfile", "-File"},
}

// resolveInterpreter 返回运行 abs 的命令和参数（参数里已含文件路径）。ok=false 表示认不出怎么运行。
func resolveInterpreter(abs, interp, root, goos string) (cmd string, args []string, ok bool) {
	ext := strings.ToLower(filepath.Ext(abs))
	switch interp {
	case "direct":
		return abs, nil, true
	case "python":
		return pythonFor(root, filepath.Dir(abs), goos), []string{abs}, true
	case "", "auto":
	default:
		if c, ok := named[interp]; ok {
			return c[0], append(append([]string{}, c[1:]...), abs), true
		}
		return interp, []string{abs}, true
	}
	if goos == "windows" {
		switch ext {
		case ".exe", ".com":
			return abs, nil, true
		case ".bat", ".cmd":
			return "cmd", []string{"/c", abs}, true
		case ".ps1":
			return "powershell", []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", abs}, true
		}
	}
	shebang := readShebang(abs)
	exec := goos != "windows" && isExecutable(abs)
	if exec && (shebang != nil || isBinary(abs)) {
		return abs, nil, true
	}
	if len(shebang) > 0 {
		c := shebangCommand(shebang)
		if len(c) > 0 {
			if isPythonName(c[0]) {
				c[0] = pythonFor(root, filepath.Dir(abs), goos)
			}
			return c[0], append(c[1:], abs), true
		}
	}
	if ext == ".py" || ext == ".pyw" {
		return pythonFor(root, filepath.Dir(abs), goos), []string{abs}, true
	}
	if c, ok := byExt[ext]; ok {
		return c[0], append(append([]string{}, c[1:]...), abs), true
	}
	if exec {
		return abs, nil, true
	}
	return "", nil, false
}

// readShebang 读第一行 #!，返回去掉 #! 后按空白拆开的词；没有返回 nil。
func readShebang(abs string) []string {
	f, err := os.Open(abs)
	if err != nil {
		return nil
	}
	defer f.Close()
	line, _ := bufio.NewReader(f).ReadString('\n')
	if !strings.HasPrefix(line, "#!") {
		return nil
	}
	return strings.Fields(strings.TrimSpace(line[2:]))
}

// shebangCommand 把 #! 的词转成要执行的命令：/usr/bin/env [-S] bash -e → bash -e；/bin/sh -e → /bin/sh -e。
func shebangCommand(words []string) []string {
	if len(words) == 0 {
		return nil
	}
	if filepath.Base(words[0]) == "env" {
		rest := words[1:]
		for len(rest) > 0 && strings.HasPrefix(rest[0], "-") {
			rest = rest[1:] // env -S、env -i 等选项
		}
		return append([]string{}, rest...)
	}
	return append([]string{}, words...)
}

func isPythonName(c string) bool {
	b := filepath.Base(c)
	return b == "python" || b == "python3" || strings.HasPrefix(b, "python3.")
}

// pythonFor 优先用工程/脚本目录里的虚拟环境。
func pythonFor(root, dir, goos string) string {
	rel := []string{".venv/bin/python", "venv/bin/python", "env/bin/python"}
	if goos == "windows" {
		rel = []string{`.venv\Scripts\python.exe`, `venv\Scripts\python.exe`}
	}
	for _, base := range []string{dir, root} {
		for _, r := range rel {
			if p := filepath.Join(base, filepath.FromSlash(r)); sdk.FileExists(p) {
				return p
			}
		}
	}
	if goos == "windows" {
		return "python"
	}
	return "python3"
}

func isExecutable(abs string) bool {
	fi, err := os.Stat(abs)
	return err == nil && !fi.IsDir() && fi.Mode().Perm()&0o111 != 0
}

// isBinary：Mach-O / ELF / PE 可执行文件头。
func isBinary(abs string) bool {
	f, err := os.Open(abs)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4)
	if n, _ := f.Read(head); n < 4 {
		return false
	}
	for _, magic := range [][]byte{{0xcf, 0xfa, 0xed, 0xfe}, {0xce, 0xfa, 0xed, 0xfe}, {0xca, 0xfe, 0xba, 0xbe}, {0xfe, 0xed, 0xfa, 0xcf}, {0x7f, 'E', 'L', 'F'}} {
		if bytes.Equal(head, magic) {
			return true
		}
	}
	return head[0] == 'M' && head[1] == 'Z'
}

// ---------------------------------------------------------------------------
// 自定义命令

// shellCommand 用 shell 执行一段（可多行的）命令。
func shellCommand(shell, command, goos, userShell string) (string, []string) {
	switch shell {
	case "cmd":
		return "cmd", []string{"/c", command}
	case "pwsh":
		return "pwsh", []string{"-NoProfile", "-Command", command}
	case "bash", "zsh", "sh":
		return shell, []string{"-c", command}
	}
	if goos == "windows" {
		// 有 PowerShell 7 就用它（支持 && ||）；Windows PowerShell 5.1 不认 &&，退回 cmd，多行命令逐行执行
		if hasPwsh() {
			return "pwsh", []string{"-NoProfile", "-Command", command}
		}
		return "cmd", []string{"/d", "/s", "/c", cmdLines(command)}
	}
	if userShell == "" {
		userShell = "/bin/sh"
	}
	return userShell, []string{"-c", command}
}

// commandDisplay：控制台第一行显示的命令（多行时显示第一行并注明行数）。
func commandDisplay(ctx *sdk.Context, command string) string {
	lines := strings.Split(strings.TrimSpace(command), "\n")
	if len(lines) == 1 {
		return lines[0]
	}
	return ctx.T("display.multiline", strings.TrimSpace(lines[0]), len(lines))
}

// ---------------------------------------------------------------------------
// 路径

func absPath(root, p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return filepath.Join(root, filepath.FromSlash(p))
}

func resolveCwd(ctx *sdk.Context, root, opt, def string) (string, error) {
	if strings.TrimSpace(opt) == "" {
		return def, nil
	}
	d := absPath(root, strings.TrimSpace(opt))
	if !sdk.DirExists(d) {
		return "", sdk.UserError("%s", ctx.T("err.cwdMissing", opt))
	}
	return d, nil
}

// displayLine：工程内的路径显示成相对路径。
func displayLine(cmd string, args []string, root string) string {
	parts := append([]string{rel(root, cmd)}, args...)
	for i := 1; i < len(parts); i++ {
		parts[i] = rel(root, parts[i])
		if strings.ContainsAny(parts[i], " \t\"'") {
			parts[i] = `"` + strings.ReplaceAll(parts[i], `"`, `\"`) + `"`
		}
	}
	return strings.Join(parts, " ")
}

func rel(root, p string) string {
	if root != "" && filepath.IsAbs(p) {
		if r, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(r, "..") {
			return filepath.ToSlash(r)
		}
	}
	return p
}
