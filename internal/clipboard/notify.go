package clipboard

import "os/exec"

func Notify(success bool) {
	event := "message"
	if !success {
		event = "dialog-error"
	}
	if path, err := exec.LookPath("canberra-gtk-play"); err == nil {
		_ = exec.Command(path, "-i", event).Start()
		return
	}
	if path, err := exec.LookPath("paplay"); err == nil {
		_ = exec.Command(path, "/usr/share/sounds/freedesktop/stereo/"+event+".oga").Start()
	}
}
