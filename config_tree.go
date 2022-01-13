package gorums

import (
	"fmt"
)

// SubConfigOption must be implemented by subconfiguration providers.
type SubConfigOption interface {
	subConfig(*RawManager) ([]RawConfiguration, error)
}

// Derive subconfigurations from the manager's base configuration.
func SubConfigurations(mgr *RawManager, opt SubConfigOption) ([]RawConfiguration, error) {
	if opt == nil {
		return nil, ConfigCreationError(fmt.Errorf("missing required subconfiguration option"))
	}
	return opt.subConfig(mgr)
}

type treeConfig struct {
	myID uint32
	bf   int
	base NodeListOption
}

func (o treeConfig) subConfig(mgr *RawManager) (configs []RawConfiguration, err error) {
	base, err := o.base.newConfig(mgr)
	if err != nil {
		return nil, err
	}
	myID, found := mgr.Node(o.myID)
	if !found {
		return nil, ConfigCreationError(fmt.Errorf("node ID %d not found", o.myID))
	}

	excludeCfg := RawConfiguration{myID}
	for levelSize := o.bf; levelSize <= len(base); levelSize *= o.bf {
		levelCfg := subLevelConfiguration(base, o.myID, levelSize)
		// compute subconfiguration at level, excluding own configuration group
		subCfg, _ := levelCfg.Except(excludeCfg).newConfig(mgr)
		configs = append(configs, subCfg)
		excludeCfg, _ = excludeCfg.And(levelCfg).newConfig(mgr)
	}
	return configs, nil
}

// subLevelConfiguration returns a configuration of size that contains myID.
func subLevelConfiguration(base RawConfiguration, myID uint32, size int) RawConfiguration {
	for i := 0; i < len(base); i += size {
		if base[i : i+size].Contains(myID) {
			return base[i : i+size]
		}
	}
	return RawConfiguration{}
}

// WithTreeConfigurations returns a SubConfigOption that can be used to create
// a set of subconfigurations representing a tree of nodes.
func WithTreeConfigurations(branchFactor int, myID uint32, base NodeListOption) SubConfigOption {
	return &treeConfig{
		bf:   branchFactor,
		myID: myID,
		base: base,
	}
}
