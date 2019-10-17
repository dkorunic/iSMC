iSMC
===

## About

`iSMC` is an Apple System Management Controller (SMC) CLI tool that attempts to query SMC for a number of well known keys and determine their type and value, classifying them into temperature, power, current, voltage, fan and battery readouts. It will also attempt to give a human-readable description of each found SMC key.

Typically various desktop and server Apple hardware should work and most definitely all Intel-based Mac computers.

## Installation

There are two ways of installing `iSMC`:

### Manual

Download your preferred flavor from [the releases](https://github.com/dkorunic/iSMC/releases/latest) page and install manually.

### Using go get

```shell
go get github.com/dkorunic/iSMC
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
  all         Display all known sensors, fans and battery status
  batt        Display battery status
  curr        Display current sensors
  fans        Display fans status
  help        Help about any command
  power       Display power sensors
  temp        Display temperature sensors
  volt        Display voltage sensors

Flags:
  -h, --help   help for iSMC

Use "iSMC [command] --help" for more information about a command.
```

Usage of all commands is self explanatory and all commands have short and long aliases (bat vs. batt vs. battery, cur vs. curr vs. current etc.). There are no flags and/or switches.

## Bugs, feature requests, etc.

Please open a PR or report an issue. Thanks!