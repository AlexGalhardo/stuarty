package service

// The store lock (SDD-005 section 13.4) is the field storeMu of svc: an import
// holds the read side from "copy the file into store/ or find it there" through
// the insert of its assets row; TrashEmpty and the sweep hold the write side.
// It lives with the other fields of svc, so a copy of a Service cannot share
// the content store without sharing the lock.
