# iSMC

[![GitHub license](https://img.shields.io/github/license/dkorunic/iSMC.svg)](https://github.com/dkorunic/iSMC/blob/master/LICENSE)
[![GitHub release](https://img.shields.io/github/release/dkorunic/iSMC.svg)](https://github.com/dkorunic/iSMC/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/dkorunic/iSMC)](https://goreportcard.com/report/github.com/dkorunic/iSMC)

## About

`iSMC` is an Apple System Management Controller (SMC) CLI tool that attempts to query SMC for a number of well known keys and determine their type and value, classifying them into temperature, power, current, voltage, fan and battery readouts. It will also attempt to give a human-readable description of each found SMC key.

Apart from regular support for regular (PPC and Intel hardware), it also supports M1 ARM-based SoC hardware which has temperature/voltage/current/power sensors connected to a HID sensor hub.

![Demo](demo.gif)

## Installation

There are two ways of installing `iSMC` (tool works only on macOS computers):

### Manual

Download your preferred flavor from [the releases](https://github.com/dkorunic/iSMC/releases/latest) page and install manually.

### Using go install

```shell
CGO_ENABLED=1 go install github.com/dkorunic/iSMC@latest
```

## Usage

Usage:

```shell
$ iSMC help
Apple SMC CLI tool that can decode and display temperature, fans, battery, power, voltage and current
information for various hardware in your Apple Mac hardware.

Usage:
  iSMC [flags]
  iSMC [command]

Available Commands:
  batt        Display battery status
  completion  Generate the autocompletion script for the specified shell
  curr        Display current sensors
  fans        Display fans status
  help        Help about any command
  power       Display power sensors
  temp        Display temperature sensors
  volt        Display voltage sensors

Flags:
  -h, --help             help for iSMC
  -o, --output string    Output format (ascii, table, json) (default "table")

Use "iSMC [command] --help" for more information about a command.
```

Usage of all commands is self explanatory and all commands have short and long aliases (bat vs. batt vs. battery, cur vs. curr vs. current etc.). There are no flags and/or switches.

## Related work

I have taken a look at many Apple SMC related projects and took inspiration from them:

- **SMCKit** Apple SMC library & tool in Swift: [github.com/beltex/SMCKit](/github.com/beltex/SMCKit)
- SMC API in pure C: [github.com/beltex/libsmc](https://github.com/beltex/libsmc)
- **iStats** Ruby Gem for Mac stats: [github.com/Chris911/iStats](https://github.com/Chris911/iStats)
- **smcFanControl** tool to control fans in Objective-C (this includes **smc-command** to query raw SMC keys): [github.com/hholtmann/smcFanControl](https://github.com/hholtmann/smcFanControl)
- **FakeSMC** Hackintosh kext: [github.com/RehabMan/OS-X-FakeSMC-kozlek](https://github.com/RehabMan/OS-X-FakeSMC-kozlek)
- **VirtualSMC** Hackintosh kext: [github.com/acidanthera/VirtualSMC](https://github.com/acidanthera/VirtualSMC)
- **osx-cpu-temp** to display CPU temperature in pure C: [github.com/lavoiesl/osx-cpu-temp](https://github.com/lavoiesl/osx-cpu-temp)
- Linux kernel **applesmc.c**: [github.com/torvalds/linux/blob/master/drivers/hwmon/applesmc.c](https://github.com/torvalds/linux/blob/master/drivers/hwmon/applesmc.c)
- low-level Go bindings for devnull SMC tool: [github.com/panotza/gosmc](https://github.com/panotza/gosmc)
- Koan-Sin Tan's M1 IOKit demo code: [https://github.com/freedomtan/sensors](https://github.com/freedomtan/sensors)
- Koan-Sin Tan's M1 exploration slides: [https://www.slideshare.net/kstan2/exploring-your-apple-m1-devices-with-open-source-tools](https://www.slideshare.net/kstan2/exploring-your-apple-m1-devices-with-open-source-tools)
- Serhiy Mytrovtsiy's MacOS **Stats** app: [https://github.com/exelban/stats](https://github.com/exelban/stats)

## Todo

Planned features:

- fetch and decode SMC key descriptions from SMC,
- generate random SMC keys and fetch/decode if available/usable,
- store those extra (random) SMC keys in permanent configuration file,
- add support for missing types (si\*, hex\_, pwm, etc.),
- various code cleanups (some parts are downright horrible).

## Bugs, feature requests, etc.

Please open a PR or report an issue. Thanks!

## Star history

[![Star History Chart](https://api.star-history.com/svg?repos=dkorunic/iSMC&type=Date)](https://star-history.com/#dkorunic/iSMC&Date)