module github.com/apppackio/apppack

go 1.16

require (
	github.com/AlecAivazis/survey/v2 v2.3.5
	github.com/TylerBrock/colorjson v0.0.0-20200706003622-8a50f05110d2 // indirect
	github.com/TylerBrock/saw v0.2.2
	github.com/apparentlymart/go-cidr v1.1.0
	github.com/aws/aws-sdk-go v1.44.25
	github.com/briandowns/spinner v1.18.1
	github.com/cli/cli/v2 v2.12.1
	github.com/cli/safeexec v1.0.0
	github.com/dustin/go-humanize v1.0.0
	github.com/fatih/color v1.13.0 // indirect
	github.com/getsentry/sentry-go v0.13.0
	github.com/google/uuid v1.3.0
	github.com/hashicorp/go-version v1.5.0
	github.com/hokaccha/go-prettyjson v0.0.0-20190818114111-108c894c2c0e // indirect
	github.com/jkueh/go-aws-console-url v0.0.1
	github.com/juju/ansiterm v0.0.0-20210929141451-8b71cc96ebdc
	github.com/kr/text v0.2.0 // indirect
	github.com/logrusorgru/aurora v2.0.3+incompatible
	github.com/mattn/go-isatty v0.0.14
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.4.0
	github.com/spf13/pflag v1.0.5
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e // indirect
	golang.org/x/term v0.0.0-20220526004731-065cf7ba2467 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0
)

replace github.com/TylerBrock/saw => github.com/apppackio/saw v0.2.3-0.20210507180802-f6559c287e6f
