# Terraform Provider for Shelly Smart Devices

This Terraform provider allows you to manage and configure Shelly Gen2 smart devices via their local network API. Built using the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework), it enables Infrastructure as Code management of your Shelly devices.

## Features

- **Device Data Source**: Query basic information from Shelly devices on your network
- **System Configuration**: Configure device names and system settings
- **Input Configuration**: Configure physical inputs on Shelly devices
- **Switch Configuration**: Configure relay switches and their behavior
- **Local Network Communication**: Direct communication with devices without cloud dependency

## Roadmap

- **Support for more components**: Will be added as I personally use them. PRs welcome for other components ;)


## Installation

### Terraform Registry (Recommended)

Once published, this provider will be available from the Terraform Registry:

```hcl
terraform {
  required_providers {
    shelly = {
      source  = "DonRobo/shelly"
      version = "~> 1.0"
    }
  }
}

provider "shelly" {
  # Configuration options
}
```

### Local Development

For local development or testing:

1. Clone this repository
2. Build the provider: `make install`
3. Use the development override in your `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "github.com/DonRobo/shelly-provider" = "/path/to/your/gopath/bin"
  }
  direct {}
}
```

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.23 (for building from source)
- Shelly devices on your local network

## Documentation

Detailed documentation for all resources and data sources is available in the [`docs/`](./docs/) directory.

## Building The Provider

1. Clone the repository
2. Enter the repository directory
3. Build the provider using the Go `install` command:

```shell
go install
```

## Development

If you wish to work on the provider, you'll first need [Go](http://www.golang.org) installed on your machine (see [Requirements](#requirements) above).

To compile the provider, run `go install`. This will build the provider and put the provider binary in the `$GOPATH/bin` directory.

### Adding Dependencies

This provider uses [Go modules](https://github.com/golang/go/wiki/Modules).
Please see the Go documentation for the most up to date information about using Go modules.

To add a new dependency `github.com/author/dependency` to your Terraform provider:

```shell
go get github.com/author/dependency
go mod tidy
```

Then commit the changes to `go.mod` and `go.sum`.


### Linting

This project uses [golangci-lint](https://github.com/golangci/golangci-lint) for linting. Install it with:

```shell
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.3.0
```

### Documentation Generation

To generate or update documentation, run:

```shell
make generate
```

### Testing

In order to run the full suite of Acceptance tests, run `make testacc`.

*Note:* Acceptance tests create real resources, and often cost money to run.

```shell
make testacc
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Acknowledgments

- Built with the [Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework)
- Uses structs from [shelly-go](https://github.com/DonRobo/shelly-go) for Shelly device communication

