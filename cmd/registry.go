package cmd

import (
	"errors"

	"github.com/jazho76/uplink/internal/config"
	"github.com/jazho76/uplink/internal/lima"
	"github.com/jazho76/uplink/internal/local"
	"github.com/jazho76/uplink/internal/remote"
	"github.com/jazho76/uplink/internal/target"
)

type selfValidating interface {
	Validate() error
}

func registry() (target.Registry, error) {
	cfg, loadErr := config.Load()
	reg, sectionErr := registryFrom(cfg)
	return reg, errors.Join(loadErr, sectionErr)
}

func registryFrom(cfg *config.File) (target.Registry, error) {
	var errs []error

	var localCfg local.Config
	if err := loadSection(cfg, local.Section, &localCfg); err != nil {
		errs = append(errs, err)
		localCfg = local.Config{}
	}

	var limaCfg lima.Config
	if err := loadSection(cfg, lima.Section, &limaCfg); err != nil {
		errs = append(errs, err)
		limaCfg = lima.Config{}
	}

	var remoteCfg remote.Config
	if err := loadSection(cfg, remote.Section, &remoteCfg); err != nil {
		errs = append(errs, err)
		remoteCfg = nil
	}

	remotes, err := remote.New(remoteCfg, cfg.Dir())
	if err != nil {
		errs = append(errs, err)
		remotes, _ = remote.New(nil, cfg.Dir())
	}

	return target.NewRegistry(
		local.New(localCfg),
		lima.New(limaCfg),
		remotes,
	), errors.Join(errs...)
}

func loadSection(cfg *config.File, name string, into any) error {
	if _, err := cfg.Section(name, into); err != nil {
		return err
	}
	if v, ok := into.(selfValidating); ok {
		return v.Validate()
	}
	return nil
}
