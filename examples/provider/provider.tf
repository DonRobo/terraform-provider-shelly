terraform {
  required_providers {
    shelly = {
      source = "github.com/DonRobo/shelly-provider"
    }
  }
}

provider "shelly" {
  # Device IP has to be set in each resource or data source.

  # Optional: the password of the devices that have authentication on.
  # You can also set it with the SHELLY_PASSWORD environment variable.
  # password = var.shelly_password
}
