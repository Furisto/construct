package api

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/furisto/construct/api/go/v1"
	"github.com/furisto/construct/api/go/v1/v1connect"
	"github.com/furisto/construct/backend/skill"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ v1connect.SkillServiceHandler = (*SkillHandler)(nil)

type SkillHandler struct {
	installer *skill.SkillManager
	v1connect.UnimplementedSkillServiceHandler
}

func NewSkillHandler(installer *skill.SkillManager) *SkillHandler {
	return &SkillHandler{
		installer: installer,
	}
}

func (h *SkillHandler) InstallSkill(ctx context.Context, req *connect.Request[v1.InstallSkillRequest]) (*connect.Response[v1.InstallSkillResponse], error) {
	source, err := skill.ParseSource(req.Msg.Source)
	if err != nil {
		return nil, apiError(connect.NewError(connect.CodeInvalidArgument, err))
	}

	opts := skill.InstallOptions{
		Force:      req.Msg.Force,
		SkillNames: req.Msg.SkillNames,
	}

	installed, err := h.installer.Install(ctx, source, opts)
	if err != nil {
		return nil, apiError(err)
	}

	protoSkills := make([]*v1.Skill, 0, len(installed))
	for _, s := range installed {
		protoSkills = append(protoSkills, convertInstalledSkillToProto(s))
	}

	return connect.NewResponse(&v1.InstallSkillResponse{
		InstalledSkills: protoSkills,
	}), nil
}

func (h *SkillHandler) ListSkills(ctx context.Context, req *connect.Request[v1.ListSkillsRequest]) (*connect.Response[v1.ListSkillsResponse], error) {
	skills, err := h.installer.List()
	if err != nil {
		return nil, apiError(err)
	}

	protoSkills := make([]*v1.Skill, 0, len(skills))
	for _, s := range skills {
		protoSkills = append(protoSkills, convertInstalledSkillToProto(s))
	}

	return connect.NewResponse(&v1.ListSkillsResponse{
		Skills: protoSkills,
	}), nil
}

func (h *SkillHandler) DeleteSkill(ctx context.Context, req *connect.Request[v1.DeleteSkillRequest]) (*connect.Response[v1.DeleteSkillResponse], error) {
	if err := h.installer.Delete(req.Msg.Name); err != nil {
		return nil, apiError(err)
	}

	return connect.NewResponse(&v1.DeleteSkillResponse{}), nil
}

func (h *SkillHandler) UpdateSkill(ctx context.Context, req *connect.Request[v1.UpdateSkillRequest]) (*connect.Response[v1.UpdateSkillResponse], error) {
	name := ""
	if req.Msg.Name != nil {
		name = *req.Msg.Name
	}

	updated, err := h.installer.Update(ctx, name)
	if err != nil {
		return nil, apiError(err)
	}

	protoSkills := make([]*v1.Skill, 0, len(updated))
	for _, s := range updated {
		protoSkills = append(protoSkills, convertInstalledSkillToProto(s))
	}

	return connect.NewResponse(&v1.UpdateSkillResponse{
		UpdatedSkills: protoSkills,
	}), nil
}

func convertInstalledSkillToProto(s *skill.InstalledSkill) *v1.Skill {
	proto := &v1.Skill{
		Name:        s.Name,
		Description: s.Description,
	}

	if !s.InstalledAt.IsZero() {
		proto.InstalledAt = timestamppb.New(s.InstalledAt)
	}
	if !s.UpdatedAt.IsZero() {
		proto.UpdatedAt = timestamppb.New(s.UpdatedAt)
	}

	if s.Git != nil {
		proto.Source = &v1.Skill_Git{
			Git: &v1.GitSource{
				Provider: convertGitProviderToProto(s.Git.Provider),
				CloneUrl: s.Git.CloneURL,
				Path:     s.Git.Path,
				Ref:      s.Git.Ref,
				TreeHash: s.Git.TreeHash,
			},
		}
	} else if s.URL != nil {
		proto.Source = &v1.Skill_Url{
			Url: &v1.UrlSource{
				Url: s.URL.URL,
			},
		}
	}

	return proto
}

func convertGitProviderToProto(p skill.GitProvider) v1.GitProvider {
	switch p {
	case skill.GitProviderGitHub:
		return v1.GitProvider_GIT_PROVIDER_GITHUB
	case skill.GitProviderGitLab:
		return v1.GitProvider_GIT_PROVIDER_GITLAB
	default:
		return v1.GitProvider_GIT_PROVIDER_UNSPECIFIED
	}
}
