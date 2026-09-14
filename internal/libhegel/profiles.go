package libhegel

import "runtime"

// SettingsNewForProfile resolves a named settings profile into an owned snapshot.
func (c *Context) SettingsNewForProfile(name string) (*Settings, error) {
	ptr, err := allocate(c, "hegel_settings_new_for_profile", func(ctx ctxT, raw *settingsT) Error {
		return c.syms.SettingsNewForProfile(ctx, name, raw)
	}, c.syms.SettingsFree)
	return (*Settings)(ptr), err
}

// SetDefaultProfile changes the process-wide default profile. A nil name clears it.
// Coordinate profile mutations with other users of this process-global configuration.
func (c *Context) SetDefaultProfile(name *string) error {
	var data *byte
	if name != nil {
		// Unlike length-delimited text, the profile name must be NUL-terminated.
		strings, _, err := cStringArrayArg([]string{*name})
		if err != nil {
			return err
		}
		data = *strings
	}
	return c.invoke("hegel_set_default_profile", func(ctx ctxT) Error {
		return c.syms.SetDefaultProfile(ctx, data)
	})
}

// RegisterProfile registers this snapshot under name in the process-wide profile registry.
func (s *Settings) RegisterProfile(ctx *Context, name string) error {
	return ctx.invoke("hegel_settings_register_profile", func(ctx ctxT) Error {
		e := s.syms.SettingsRegisterProfile(ctx, name, s.raw)
		runtime.KeepAlive(s)
		return e
	})
}

// TestLocation sets the location used for Antithesis assertion reporting.
func (s *Settings) TestLocation(ctx *Context, file string, beginLine uint32, className, function string) error {
	return ctx.invoke("hegel_settings_set_test_location", func(ctx ctxT) Error {
		e := s.syms.SettingsSetTestLocation(ctx, s.raw, file, beginLine, className, function)
		runtime.KeepAlive(s)
		return e
	})
}

// PrintBlob controls reproduction-blob output in the engine report.
func (s *Settings) PrintBlob(ctx *Context, yes bool) error {
	return ctx.invoke("hegel_settings_set_print_blob", func(ctx ctxT) Error {
		e := s.syms.SettingsSetPrintBlob(ctx, s.raw, yes)
		runtime.KeepAlive(s)
		return e
	})
}

// Block opens an indented print region sharing this handle's choice sequence.
// The block and its parent must not be used concurrently.
func (tc *TestCase) Block(ctx *Context, indent uint64) (*TestCase, error) {
	ptr, err := allocate(ctx, "hegel_test_case_block", func(ctx ctxT, raw *testCaseT) Error {
		e := tc.syms.TestCaseBlock(ctx, tc.raw, indent, raw)
		runtime.KeepAlive(tc)
		return e
	}, tc.syms.TestCaseFree)
	if ptr == nil {
		return nil, err
	}
	return &TestCase{pointer: ptr}, err
}

// SetWorker attributes subsequent output from this handle to a worker index.
func (tc *TestCase) SetWorker(ctx *Context, workerIndex int64) error {
	return ctx.invoke("hegel_test_case_set_worker", func(ctx ctxT) Error {
		e := tc.syms.TestCaseSetWorker(ctx, tc.raw, workerIndex)
		runtime.KeepAlive(tc)
		return e
	})
}

// LabelFromName derives a stable span identity from a generator's qualified name.
func (c *Context) LabelFromName(name string) (Label, error) {
	var value uint64
	err := c.invoke("hegel_label_from_name", func(ctx ctxT) Error {
		return c.syms.LabelFromName(ctx, name, &value)
	})
	return Label(value), err
}

// LabelCombine derives an order-sensitive span identity from component labels.
func (c *Context) LabelCombine(labels []Label) (Label, error) {
	var value uint64
	err := c.invoke("hegel_label_combine", func(ctx ctxT) Error {
		return c.syms.LabelCombine(ctx, slicePtr(labels), uint64(len(labels)), &value)
	})
	return Label(value), err
}

// GetSeed returns the configured seed and whether one was explicitly set.
func (s *Settings) GetSeed(ctx *Context) (uint64, bool, error) {
	var seed uint64
	var hasSeed bool
	err := ctx.invoke("hegel_settings_get_seed", func(ctx ctxT) Error {
		e := s.syms.SettingsGetSeed(ctx, s.raw, &seed, &hasSeed)
		runtime.KeepAlive(s)
		return e
	})
	return seed, hasSeed, err
}

// GetDatabase returns nil for the default database, a pointer to "" for disabled,
// or a pointer to the configured path. The borrowed native string is copied.
func (s *Settings) GetDatabase(ctx *Context) (*string, error) {
	var data *byte
	err := ctx.invoke("hegel_settings_get_database", func(ctx ctxT) Error {
		e := s.syms.SettingsGetDatabase(ctx, s.raw, &data)
		runtime.KeepAlive(s)
		return e
	})
	if err != nil || data == nil {
		return nil, err
	}
	value := goString(data)
	runtime.KeepAlive(s)
	return &value, nil
}

// GetTestCases reads the effective test cases setting.
func (s *Settings) GetTestCases(ctx *Context) (uint64, error) {
	var value uint64
	err := ctx.invoke("hegel_settings_get_test_cases", func(ctx ctxT) Error {
		e := s.syms.SettingsGetTestCases(ctx, s.raw, &value)
		runtime.KeepAlive(s)
		return e
	})
	return uint64(value), err
}

// GetVerbosity reads the effective verbosity setting.
func (s *Settings) GetVerbosity(ctx *Context) (Verbosity, error) {
	var value int32
	err := ctx.invoke("hegel_settings_get_verbosity", func(ctx ctxT) Error {
		e := s.syms.SettingsGetVerbosity(ctx, s.raw, &value)
		runtime.KeepAlive(s)
		return e
	})
	return Verbosity(value), err
}

// GetDerandomize reads the effective derandomize setting.
func (s *Settings) GetDerandomize(ctx *Context) (bool, error) {
	var value bool
	err := ctx.invoke("hegel_settings_get_derandomize", func(ctx ctxT) Error {
		e := s.syms.SettingsGetDerandomize(ctx, s.raw, &value)
		runtime.KeepAlive(s)
		return e
	})
	return bool(value), err
}

// GetPhases reads the effective phases setting.
func (s *Settings) GetPhases(ctx *Context) (Phase, error) {
	var value uint32
	err := ctx.invoke("hegel_settings_get_phases", func(ctx ctxT) Error {
		e := s.syms.SettingsGetPhases(ctx, s.raw, &value)
		runtime.KeepAlive(s)
		return e
	})
	return Phase(value), err
}

// GetSuppressHealthCheck reads the effective suppress health check setting.
func (s *Settings) GetSuppressHealthCheck(ctx *Context) (HealthCheck, error) {
	var value uint32
	err := ctx.invoke("hegel_settings_get_suppress_health_check", func(ctx ctxT) Error {
		e := s.syms.SettingsGetSuppressHealthCheck(ctx, s.raw, &value)
		runtime.KeepAlive(s)
		return e
	})
	return HealthCheck(value), err
}

// GetReportMultipleFailures reads the effective report multiple failures setting.
func (s *Settings) GetReportMultipleFailures(ctx *Context) (bool, error) {
	var value bool
	err := ctx.invoke("hegel_settings_get_report_multiple_failures", func(ctx ctxT) Error {
		e := s.syms.SettingsGetReportMultipleFailures(ctx, s.raw, &value)
		runtime.KeepAlive(s)
		return e
	})
	return bool(value), err
}

// GetShowStatistics reads the effective show statistics setting.
func (s *Settings) GetShowStatistics(ctx *Context) (bool, error) {
	var value bool
	err := ctx.invoke("hegel_settings_get_show_statistics", func(ctx ctxT) Error {
		e := s.syms.SettingsGetShowStatistics(ctx, s.raw, &value)
		runtime.KeepAlive(s)
		return e
	})
	return bool(value), err
}

// GetPrintBlob reads the effective print blob setting.
func (s *Settings) GetPrintBlob(ctx *Context) (bool, error) {
	var value bool
	err := ctx.invoke("hegel_settings_get_print_blob", func(ctx ctxT) Error {
		e := s.syms.SettingsGetPrintBlob(ctx, s.raw, &value)
		runtime.KeepAlive(s)
		return e
	})
	return bool(value), err
}

// GetBackend reads the effective backend setting.
func (s *Settings) GetBackend(ctx *Context) (Backend, error) {
	var value int32
	err := ctx.invoke("hegel_settings_get_backend", func(ctx ctxT) Error {
		e := s.syms.SettingsGetBackend(ctx, s.raw, &value)
		runtime.KeepAlive(s)
		return e
	})
	return Backend(value), err
}
