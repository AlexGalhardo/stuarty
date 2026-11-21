package service

import (
	"context"
	"errors"
	"testing"
)

type productionsStub struct {
	ProductionRepo
	getErr, ofErr error
	owner         string
}

func (p productionsStub) Get(context.Context, string) (ProductionHead, error) {
	return ProductionHead{}, p.getErr
}

func (p productionsStub) ProjectOfTimeline(context.Context, string) (string, error) {
	return p.owner, p.ofErr
}

type productionsOf struct{ repo ProductionRepo }

func (p productionsOf) Productions() ProductionRepo { return p.repo }

// A failed read is not "this id names no timeline": answering with the id would skip the ceiling of the project and
// let a rule stored with the id of a later timeline match (D-132).
func TestCostProject_AFailedReadIsAnErrorNotTheID(t *testing.T) {
	boom := errors.New("database is locked")
	ctx := context.Background()

	for name, stub := range map[string]productionsStub{
		"reading the project":  {getErr: boom},
		"reading the timeline": {getErr: ErrNotFound, ofErr: boom},
	} {
		if got, err := costProject(ctx, productionsOf{stub}, "prj_later"); !errors.Is(err, boom) {
			t.Errorf("%s: costProject = %q, %v; want the error", name, got, err)
		}
	}

	if got, err := costProject(
		ctx, productionsOf{productionsStub{getErr: ErrNotFound, owner: "prj_owner"}}, "prj_later",
	); err != nil || got != "prj_owner" {
		t.Errorf("a later timeline = %q, %v; want its project", got, err)
	}

	if got, err := costProject(
		ctx, productionsOf{productionsStub{getErr: ErrNotFound, ofErr: ErrNotFound}}, "prj_nothing",
	); err != nil || got != "prj_nothing" {
		t.Errorf("an id of nothing = %q, %v; want the id", got, err)
	}

	if got, err := costProject(ctx, productionsOf{productionsStub{owner: "prj_other"}}, "prj_project"); err != nil ||
		got != "prj_project" {
		t.Errorf("an id that names a project = %q, %v; want that project", got, err)
	}
}
