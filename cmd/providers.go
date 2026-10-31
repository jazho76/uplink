package cmd

import (
	"github.com/jazho76/uplink/internal/inventory"
	"github.com/jazho76/uplink/internal/provider/docker"
	"github.com/jazho76/uplink/internal/provider/lima"
	"github.com/jazho76/uplink/internal/provider/local"
)

var providers = []inventory.Provider{local.Provider{}, lima.Provider{}, docker.Provider{}}
