module github.com/fuweid/go-dmflakey/contrib

go 1.20

replace github.com/fuweid/go-dmflakey => ../

require (
	github.com/fuweid/go-dmflakey v0.0.0-00010101000000-000000000000
	github.com/stretchr/testify v1.8.4
	go.etcd.io/bbolt v1.3.8
	golang.org/x/sys v0.14.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
