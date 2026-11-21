// Package account holds the pure rules of the local account (ADR-0049, SDD-007
// section 3.9): what a username, an e-mail address and a password must look like,
// and the reasons a field is refused. It does no I/O, derives no key and stores
// nothing; the keys and the storage are in service/account. Every surface (the
// pages, the CLI, the API) gets the same answer from here.
package account
