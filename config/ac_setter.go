package config

type AccessControlSetter struct {
	ErrorHandler []*ErrorHandler `hcl:"error_handler,block"`
}

func (acs *AccessControlSetter) Set(ehConf *ErrorHandler) {
	acs.ErrorHandler = append(acs.ErrorHandler, ehConf)
}
