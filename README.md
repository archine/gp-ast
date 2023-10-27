![](https://img.shields.io/badge/version-1.x-green.svg) &nbsp; ![](https://img.shields.io/badge/builder-success-green.svg) &nbsp;

> Go gin-plus框架字节码工具

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

**gin-plus版本需 3.x**

```go
//go:generate gp-ast
func main() {
    application.Default().Run()
}
```
