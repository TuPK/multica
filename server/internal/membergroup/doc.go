// Package membergroup holds the account groups of platform management spec
// 7b.2: a fixed catalogue of six permission keys, the rules a group's name and
// keys must meet, the effective permissions of a person, and the store that
// reads and writes the two group tables.
//
// A group only adds permissions to the ones a role implies. Managing groups is
// not a key of the catalogue, so it stays with owner and admin.
package membergroup
