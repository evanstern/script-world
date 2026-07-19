package main

import "errors"

var errNotImplemented = errors.New("not implemented")

func cmdNew(args []string) error              { return errNotImplemented }
func cmdDaemon(args []string) error           { return errNotImplemented }
func cmdStart(args []string) error            { return errNotImplemented }
func cmdStop(args []string) error             { return errNotImplemented }
func cmdStatus(args []string) error           { return errNotImplemented }
func cmdAttach(args []string) error           { return errNotImplemented }
func cmdTail(args []string) error             { return errNotImplemented }
func cmdTimeCtl(cmd string, args []string) error { return errNotImplemented }
func cmdSpeed(args []string) error            { return errNotImplemented }
