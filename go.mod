module github.com/apppackio/apppack

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.3.5
	github.com/TylerBrock/colorjson v0.0.0-20200706003622-8a50f05110d2 // indirect
	github.com/TylerBrock/saw v0.2.2
	github.com/apparentlymart/go-cidr v1.1.0
	github.com/aws/aws-sdk-go v1.44.61
	github.com/briandowns/spinner v1.18.1
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/color v1.13.0 // indirect
	github.com/getsentry/sentry-go v0.13.0
	github.com/google/uuid v1.3.0
	github.com/hokaccha/go-prettyjson v0.0.0-20190818114111-108c894c2c0e // indirect
	github.com/jkueh/go-aws-console-url v0.0.1
	github.com/juju/ansiterm v1.0.0
	github.com/kr/text v0.2.0 // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mum4k/termdash v0.17.0
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/sirupsen/logrus v1.9.0
	github.com/spf13/cobra v1.5.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/crypto v0.0.0-20220722155217-630584e8d5aa // indirect
	golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f // indirect
	golang.org/x/term v0.0.0-20220722155259-a9ba230a4035 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

replace github.com/TylerBrock/saw => github.com/apppackio/saw v0.2.3-0.20210507180802-f6559c287e6f
