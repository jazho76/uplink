package target

type Environment struct {
	Profile  Profile
	Readings []Reading
	Tools    []string
	Sessions []Session
	Missing  MissingSections
}

type MissingSections struct {
	Profile  bool
	Stats    bool
	Tools    bool
	Sessions bool
}
