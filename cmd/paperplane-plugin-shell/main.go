// paperplane-plugin-shell 是「脚本与可执行文件」插件的独立可执行入口。
package main

import (
	sdk "github.com/lemon-paperplane/lemon-paperplane-main/lemon-paperplane-main-plugin-sdk"
	shell "github.com/lemon-paperplane/lemon-paperplane-plugin-shell/plugin"
)

func main() { sdk.Serve(shell.New()) }
