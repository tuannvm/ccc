package main

import "sync"

// Auth state for OTP permission approval
var authInProgress sync.Mutex
var authWaitingCode bool
var otpAttempts = make(map[string]int) // session -> failed attempts
