module github.com/nanmu42/gzip/examples/gin

go 1.16

replace (
	github.com/nanmu42/gzip/adaptors/gin/v2 => ../../adaptors/gin
	github.com/nanmu42/gzip/v2 => ../..
)

require (
	github.com/gin-gonic/gin v1.6.3
	github.com/nanmu42/gzip/adaptors/gin/v2 v2.0.0-00010101000000-000000000000
	github.com/nanmu42/gzip/v2 v2.0.0-00010101000000-000000000000
)