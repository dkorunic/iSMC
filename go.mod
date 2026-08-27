module github.com/dkorunic/iSMC

go 1.27

replace github.com/dkorunic/iSMC/gosmc => ./gosmc

require (
	github.com/dkorunic/iSMC/gosmc v0.0.0-20260413130435-fb3be841d2e6
	github.com/fvbommel/sortorder v1.1.0
	github.com/jedib0t/go-pretty/v6 v6.8.3
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.12.1
)

require (
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-runewidth v0.0.28 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)
