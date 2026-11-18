# lemon-paperplane-plugin-shell

Lemon Paperplane 的「脚本与可执行文件」插件（插件 id `shell`），内置随 App 安装。

把任意脚本、可执行文件或一段自定义命令作为运行配置，可以加入工作区的「启动」一键运行：

- **脚本 / 可执行文件**（`script`）：文件树里右键「添加到启动」，或调用 `launch.addFile`。
  解释器默认自动选择：有执行权限且带 `#!` 的直接运行；带 `#!` 的按它指定的解释器运行（`/usr/bin/env -S …` 也认）；
  否则按扩展名（`.sh` 用 bash、`.py` 优先用工程里的 `.venv` / `venv`、`.js` 用 node、`.jar` 用 `java -jar` 等）。
  没有执行权限时给出诊断，可一键 `chmod +x` 或改用 sh 运行。
- **自定义命令**（`command`）：在「启动」里点「添加命令」，填名称、执行目录、命令（CodeMirror 输入框，可多行），或调用 `launch.addCommand`。
  用你的默认 shell（Windows 上是 PowerShell）`-c` 执行。

两种配置都标了 `anyModule`：任何模块（包括没识别出技术的工程根目录）都能建；插件不参与识别分析，只在运行这类配置时才启动。

## 构建与测试

```bash
./build.sh          # 构建 bin/paperplane-plugin-shell
go vet ./... && go test ./...
```
