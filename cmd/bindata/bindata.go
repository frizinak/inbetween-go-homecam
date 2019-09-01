package main

import "github.com/go-bindata/go-bindata"

func main() {
	c := bindata.NewConfig()
	c.Package = "bound"
	c.Output = "bound/bound.go"
	c.Input = []bindata.InputConfig{
		bindata.InputConfig{"bind", true},
	}
	c.Prefix = "bind"
	c.NoMetadata = true
	if err := bindata.Translate(c); err != nil {
		panic(err)
	}
}
