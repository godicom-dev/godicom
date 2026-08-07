module github.com/godicom-dev/godicom/scripts/bench_compare

go 1.26

require (
	github.com/godicom-dev/godicom v0.25.1
	github.com/godicom-dev/gonetdicom v0.14.0
)

require (
	github.com/ebitengine/purego v0.10.1 // indirect
	github.com/godicom-dev/golibjpeg v1.2.1 // indirect
	github.com/godicom-dev/goopenjpeg v1.1.1 // indirect
	github.com/godicom-dev/gorle v1.0.1 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.38.0 // indirect
)

replace github.com/godicom-dev/godicom => ../..

replace github.com/godicom-dev/gonetdicom => ../../../gonetdicom
