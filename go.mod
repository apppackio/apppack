module github.com/apppackio/apppack

go 1.25.4

require (
	github.com/AlecAivazis/survey/v2 v2.3.7
	github.com/TylerBrock/saw v0.2.2
	github.com/apparentlymart/go-cidr v1.1.0
	github.com/aws/aws-sdk-go v1.55.8
	github.com/briandowns/spinner v1.23.2
	github.com/dustin/go-humanize v1.0.1
	github.com/getsentry/sentry-go v0.36.2
	github.com/google/uuid v1.6.0
	github.com/jkueh/go-aws-console-url v0.0.1
	github.com/juju/ansiterm v1.0.0
	github.com/logrusorgru/aurora v2.0.3+incompatible
	github.com/mattn/go-isatty v0.0.20
	github.com/mum4k/termdash v0.20.0
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/sirupsen/logrus v1.9.3
	github.com/spf13/cobra v1.10.1
	github.com/spf13/pflag v1.0.10
	github.com/stretchr/testify v1.11.1
	gopkg.in/square/go-jose.v2 v2.6.0
)

require (
	github.com/aws/session-manager-plugin v0.0.0-20250205214155-b2b0bcd769d1
	github.com/cli/cli/v2 v2.83.0
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/cihub/seelog v0.0.0-20170130134532-f561c5e57575 // indirect
	github.com/cli/go-gh/v2 v2.13.0 // indirect
	github.com/cli/shurcooL-graphql v0.0.4 // indirect
	github.com/clipperhouse/stringish v0.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.3.0 // indirect
	github.com/eiannone/keyboard v0.0.0-20220611211555-0d226195f203 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/henvic/httpretty v0.1.4 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/thlib/go-timezone-local v0.0.7 // indirect
	github.com/twinj/uuid v1.0.0 // indirect
	github.com/xtaci/smux v1.5.35 // indirect
	golang.org/x/sync v0.18.0 // indirect
)

require (
	github.com/TylerBrock/colorjson v0.0.0-20200706003622-8a50f05110d2 // indirect
	github.com/cli/safeexec v1.0.1
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/gdamore/tcell/v2 v2.9.0 // indirect
	github.com/hashicorp/go-version v1.7.0
	github.com/hokaccha/go-prettyjson v0.0.0-20190818114111-108c894c2c0e // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/lunixbochs/vtclean v1.0.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-runewidth v0.0.19 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	golang.org/x/crypto v0.43.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/term v0.36.0 // indirect
	golang.org/x/text v0.30.0
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/TylerBrock/saw => github.com/apppackio/saw v0.2.3-0.20210507180802-f6559c287e6f

// https://github.com/aws/session-manager-plugin/issues/73
replace github.com/twinj/uuid => github.com/twinj/uuid v0.0.0-20151029044442-89173bcdda19
