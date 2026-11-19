package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func write(t *testing.T, dir, name, body string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(dir, name)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolveInterpreter(t *testing.T) {
	root := t.TempDir()
	execSh := write(t, root, "run.sh", "#!/usr/bin/env bash\necho hi\n", 0o755)
	plainSh := write(t, root, "plain.sh", "echo hi\n", 0o644)
	envS := write(t, root, "tool", "#!/usr/bin/env -S node --no-warnings\n", 0o644)
	py := write(t, root, "scripts/job.py", "print(1)\n", 0o644)
	venvPy := write(t, root, ".venv/bin/python", "", 0o755)
	pyShebang := write(t, root, "scripts/x", "#!/usr/bin/env python3\n", 0o644)
	unknown := write(t, root, "data.bin", "\x00\x01", 0o644)
	cases := []struct {
		file, interp string
		cmd          string
		args         []string
		ok           bool
	}{
		{execSh, "auto", execSh, nil, runtime.GOOS != "windows"},      // 有执行权限且带 #!：直接运行（Windows 没有执行权限位，跳过）
		{plainSh, "auto", "bash", []string{plainSh}, true},            // 没权限、没 #!：按扩展名
		{envS, "auto", "node", []string{"--no-warnings", envS}, true}, // env -S
		{py, "auto", venvPy, []string{py}, true},                      // .py 优先工程虚拟环境
		{pyShebang, "auto", venvPy, []string{pyShebang}, true},        // #! python3 也用虚拟环境
		{plainSh, "zsh", "zsh", []string{plainSh}, true},
		{plainSh, "direct", plainSh, nil, true},
		{unknown, "auto", "", nil, false},
	}
	for _, c := range cases {
		if c.file == execSh && c.interp == "auto" && runtime.GOOS == "windows" {
			continue
		}
		cmd, args, ok := resolveInterpreter(c.file, c.interp, root, "darwin")
		if cmd != c.cmd || !reflect.DeepEqual(args, c.args) || ok != c.ok {
			t.Errorf("%s/%s: got %q %q %v want %q %q %v", filepath.Base(c.file), c.interp, cmd, args, ok, c.cmd, c.args, c.ok)
		}
	}
	if cmd, args, _ := resolveInterpreter(write(t, root, "a.bat", "", 0o644), "auto", root, "windows"); cmd != "cmd" || args[0] != "/c" {
		t.Errorf("windows bat: %q %q", cmd, args)
	}
}

func TestShellCommand(t *testing.T) {
	if c, a := shellCommand("auto", "npm i\nnpm run dev", "darwin", "/bin/zsh"); c != "/bin/zsh" || a[0] != "-c" || a[1] != "npm i\nnpm run dev" {
		t.Errorf("auto: %q %q", c, a)
	}
	old := hasPwsh
	defer func() { hasPwsh = old }()
	hasPwsh = func() bool { return true }
	if c, _ := shellCommand("auto", "dir", "windows", ""); c != "pwsh" {
		t.Errorf("windows auto (pwsh): %q", c)
	}
	hasPwsh = func() bool { return false }
	if c, a := shellCommand("auto", "npm i\r\nnpm run dev", "windows", ""); c != "cmd" || a[len(a)-1] != "npm i & npm run dev" {
		t.Errorf("windows auto (cmd): %q %q", c, a)
	}
	if c, a := shellCommand("bash", "echo 1", "linux", "/bin/zsh"); c != "bash" || a[1] != "echo 1" {
		t.Errorf("bash: %q %q", c, a)
	}
}

func TestDisplayLine(t *testing.T) {
	root, script := "/p", "/p/scripts/a b.sh"
	if runtime.GOOS == "windows" {
		root, script = `C:\p`, `C:\p\scripts\a b.sh`
	}
	if got := displayLine("bash", []string{script, "--x"}, root); got != `bash "scripts/a b.sh" --x` {
		t.Errorf("display: %s", got)
	}
}
