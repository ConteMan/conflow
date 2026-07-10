package app

import (
	"context"
	"errors"
	"slices"

	"github.com/ConteMan/conflow/internal/project"
)

var ErrLastEnvironment = errors.New("cannot delete the last environment")

type Service struct {
	projects *project.Store
}

func Initialize(workspace string) (string, error) {
	return project.CreateExample(workspace)
}

func Open(workspace string) (*Service, error) {
	store, err := project.Open(workspace)
	if err != nil {
		return nil, err
	}
	return &Service{projects: store}, nil
}

func (s *Service) Snapshot(_ context.Context) (project.Snapshot, error) {
	return s.projects.Snapshot()
}

func (s *Service) UpdateProject(_ context.Context, expectedRevision uint64, metadata project.Project) (project.Snapshot, error) {
	return s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		manifest.Project = metadata
		return nil
	})
}

func (s *Service) CreateEnvironment(_ context.Context, expectedRevision uint64, environment project.Environment) (project.Snapshot, project.Environment, error) {
	snapshot, err := s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		if slices.ContainsFunc(manifest.Environments, func(current project.Environment) bool {
			return current.ID == environment.ID
		}) {
			return project.ErrAlreadyExists
		}
		manifest.Environments = append(manifest.Environments, environment)
		return nil
	})
	return snapshot, environment, err
}

func (s *Service) GetEnvironment(ctx context.Context, environmentID string) (project.Snapshot, project.Environment, error) {
	snapshot, err := s.Snapshot(ctx)
	if err != nil {
		return project.Snapshot{}, project.Environment{}, err
	}
	for _, environment := range snapshot.Manifest.Environments {
		if environment.ID == environmentID {
			return snapshot, environment, nil
		}
	}
	return snapshot, project.Environment{}, project.ErrNotFound
}

func (s *Service) UpdateEnvironment(_ context.Context, expectedRevision uint64, environmentID string, replacement project.Environment) (project.Snapshot, project.Environment, error) {
	replacement.ID = environmentID
	snapshot, err := s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		for index := range manifest.Environments {
			if manifest.Environments[index].ID == environmentID {
				manifest.Environments[index] = replacement
				return nil
			}
		}
		return project.ErrNotFound
	})
	return snapshot, replacement, err
}

func (s *Service) DeleteEnvironment(_ context.Context, expectedRevision uint64, environmentID string) (project.Snapshot, error) {
	return s.projects.Update(expectedRevision, func(manifest *project.Manifest) error {
		if len(manifest.Environments) == 1 && manifest.Environments[0].ID == environmentID {
			return ErrLastEnvironment
		}
		for index := range manifest.Environments {
			if manifest.Environments[index].ID == environmentID {
				manifest.Environments = slices.Delete(manifest.Environments, index, index+1)
				return nil
			}
		}
		return project.ErrNotFound
	})
}
