# go-execplus  [![Build Status](https://travis-ci.org/Originate/go-execplus.svg?branch=master)](https://travis-ci.org/Originate/go-execplus) [![Go Report Card](https://goreportcard.com/badge/github.com/Originate/go-execplus)](https://goreportcard.com/report/github.com/Originate/go-execplus)

An abstraction around [os/exec.Cmd](https://golang.org/pkg/os/exec/#Cmd)
that allows you to:

* wait for specific text to appear in the output
* receive output chunks via a channel

Requires Go 1.7 or above.

See the tests for examples
