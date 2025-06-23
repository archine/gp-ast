package enum

const (
	MvcFlag  = 1 << iota // Flag indicating MVC structure
	BeanFlag             // Flag indicating Bean structure

	BeanInitFile = "gp_bean_init.go" // Generated Bean initialization file
	ApiDefFile   = ".gp_api.json"    // Generated API definition file

	BeanImportPath = "github.com/archine/gin-plus/v4/ioc" // Import path for Bean functionality
	MvcImportPath  = "github.com/archine/gin-plus/v4/mvc" // Import path for MVC functionality
)
