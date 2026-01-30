package local

import "godoit.dev/doit/target"

type Local struct{}

func (Local) Kind() string                        { return "local" }
func (Local) NewConfig() any                      { return &Config{} }
func (Local) Create(_ any) (target.Target, error) { return POSIXTarget{}, nil }

type Config struct{}
