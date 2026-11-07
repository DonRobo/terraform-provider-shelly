variable "shelly_password" {
  type      = string
  sensitive = true
}

# Set the password on the device. Use the same value as the provider
# "password", so that the other resources can connect to the device.
resource "shelly_auth" "living_room" {
  ip       = "192.168.1.100"
  password = var.shelly_password
}
