resource "shelly_webhook" "example" {
  ip    = "192.168.1.100"
  cid   = 0
  event = "input.toggle_on"
  name  = "Notify on toggle"
  urls  = ["http://example.com/notify"]
}
