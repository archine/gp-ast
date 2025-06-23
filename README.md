![](https://img.shields.io/badge/version-1.x-green.svg) &nbsp; ![](https://img.shields.io/badge/builder-success-green.svg) &nbsp;

> [Gin-Plus-v3](https://github.com/archine/gin-plus) 配套工具，提供 AST 解析功能。

### 1、安装

确保本机 go bin 目录存在系统环境变量中

```bash
go install github.com/archine/gp-ast@latest
```

### 2、参数

| 命令       | 描述                       |
|----------|--------------------------|
| -root    | API文件根目录，默认 controller   |
| -context | 程序上下文路径，默认 /             |
| -f       | 需要过滤的目录,英文逗号分割, 如：dto,vo |
| -h       | 查看使用文档                   |

### 3、使用

在 main 文件中添加 ``go:generate gp-ast`` 注释

```go
package main

//go:generate gp-ast
func main() {
	application.Default().Run()
}
```

然后在 main.go 所在目录下执行以下命令：

```bash
go generate
```

### 4、输出

生成的文件会存在于 root 所在目录下

- **controller_init.go**: API 接口信息
