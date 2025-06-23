![](https://img.shields.io/badge/version-2.x-green.svg) &nbsp; ![](https://img.shields.io/badge/builder-success-green.svg) &nbsp;

> [Gin-Plus-v4](https://github.com/archine/gin-plus) 配套工具，提供 AST 解析功能。

### 1、安装

确保 GoPath 中的 bin 目录存在系统环境变量中，或者安装后移动到系统环境变量配置了的目录中。

```bash
go install github.com/archine/gp-ast/v2@v2.0.0
```

### 2、参数

| 命令         | 描述                                                      |
|------------|---------------------------------------------------------|
| -context   | 程序上下文路径，默认 '/'                                        |
| -scan_pkg  | 要扫描的包名，默认为 '.' 表示扫描当前绝对路径下的所有包。多个包名，用逗号分隔                 |
| -scan_skip | 扫描时要跳过的包名，默认为 'dto,vo' 当指定的目录位于顶层时，它的子目录也将被忽略。多个包名，用逗号分隔 |
| -h         | 查看命令                                                    |

### 3、使用

在 main 文件中添加 ``go:generate gp-ast`` 注释, 命令可选

```go
package main

//go:generate gp-ast -context /xxx -scan_pkg .
func main() {
	application.Default().Run()
}
```

然后在 main.go 所在目录下执行以下命令：

```bash
go generate
```

### 4、输出

生成的文件会存在于 main.go 所在目录下

- **.gp_api.json**: API 接口信息
- **gp_bean_init.go**: Bean 初始化代码
