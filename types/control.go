package types

// ControlCheckKeyArgs : Parameters of function control.CheckKey (ignore context)
type ControlCheckKeyArgs struct {
	Key string
}

// ControlCheckKeyReply : Response of function control.CheckKey
type ControlCheckKeyReply struct {
	Account *string
	KeyID   *string
}

// CheckAccountPausedArgs : Parameters of function control.CheckAccountPaused (ignore context)
type CheckAccountPausedArgs struct {
	Account string
}

// CheckAccountPausedReply : Response of function control.CheckAccountPaused
type CheckAccountPausedReply struct {
	IsPaused bool
}
