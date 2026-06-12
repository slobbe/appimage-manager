package app

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"aim/internal/domain"
)

type DummyService struct {
	config Config
}

func NewDummyService(config Config) *DummyService {
	return &DummyService{
		config: config,
	}
}

func (s *DummyService) Add(ctx context.Context, req AddRequest) (AddResult, error) {
	activity := req.Activity
	if activity == nil {
		activity = NoopActivityReporter{}
	}

	if req.GitHubRepo != "" {
		return s.addFromGitHub(ctx, activity, req.GitHubRepo)
	}

	return s.integrateLocal(ctx, activity, req.Path)
}

func (s *DummyService) addFromGitHub(ctx context.Context, activity ActivityReporter, repo string) (AddResult, error) {
	check := activity.Start(ctx, Activity{
		Kind: ActivityKindCheckingGitHub,
		Repo: repo,
	})
	if err := wait(ctx, 900*time.Millisecond); err != nil {
		check.Fail(err)
		return AddResult{}, err
	}
	check.Done("Checked " + repo)

	const mb = 1024 * 1024
	const total = 140 * mb
	download := activity.Start(ctx, Activity{
		Kind:      ActivityKindDownloading,
		AssetName: "asset.AppImage",
		Total:     total,
		Unit:      ActivityUnitBytes,
	})
	for downloaded := int64(0); downloaded < total; downloaded += 10 * mb {
		if err := wait(ctx, 120*time.Millisecond); err != nil {
			download.Fail(err)
			return AddResult{}, err
		}
		download.Advance(10 * mb)
	}
	download.Done("Downloaded asset.AppImage")

	return s.integrateLocal(ctx, activity, "asset.AppImage")
}

func (s *DummyService) integrateLocal(ctx context.Context, activity ActivityReporter, path string) (AddResult, error) {
	integrate := activity.Start(ctx, Activity{
		Kind: ActivityKindIntegrating,
		Path: path,
	})
	if err := wait(ctx, 900*time.Millisecond); err != nil {
		integrate.Fail(err)
		return AddResult{}, err
	}
	integrate.Done("Integrated " + filepath.Base(path))

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if name == "" {
		name = "AppImage"
	}

	source := domain.NewLocalSource(path, time.Now())
	updateSource := domain.UpdateSource{}
	if strings.HasPrefix(path, "asset") {
		updateSource = domain.NewGitHubUpdateSource("owner/repo", false)
		source = domain.NewGitHubReleaseSource(updateSource.Repo, "v0.0.0", "asset.AppImage", "", 0, time.Now())
	}

	return AddResult{App: domain.NewApp(domain.AppInput{
		Name:         name,
		AppImagePath: path,
		Source:       source,
		UpdateSource: updateSource,
	})}, nil
}

func wait(ctx context.Context, duration time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(duration):
		return nil
	}
}

func (s *DummyService) Remove(ctx context.Context, req RemoveRequest) error {
	activity := req.Activity
	if activity == nil {
		activity = NoopActivityReporter{}
	}

	remove := activity.Start(ctx, Activity{
		Kind:  ActivityKindRemoving,
		AppID: req.Name,
	})

	if err := wait(ctx, 1200*time.Millisecond); err != nil {
		remove.Fail(err)
		return err
	}
	remove.Done("Removed AppImage")

	return nil
}

func (s *DummyService) SetUpdateSource(ctx context.Context, req SetUpdateSourceRequest) (SetUpdateSourceResult, error) {
	updateSource := domain.UpdateSource{}
	if req.GitHubRepo != "" {
		updateSource = domain.NewGitHubUpdateSource(req.GitHubRepo, req.Prerelease)
	} else if req.Embedded {
		updateSource = domain.NewEmbeddedUpdateSource("gh-releases-zsync|owner|repo|latest|asset.AppImage.zsync")
	}

	return SetUpdateSourceResult{ID: req.ID, UpdateSource: updateSource}, nil
}

func (s *DummyService) UnsetUpdateSource(ctx context.Context, req UnsetUpdateSourceRequest) error {
	return nil
}

func (s *DummyService) Update(ctx context.Context, req UpdateRequest) (UpdateResult, error) {
	activity := req.Activity
	if activity == nil {
		activity = NoopActivityReporter{}
	}

	check := activity.Start(ctx, Activity{
		Kind: ActivityKindCheckingUpdates,
	})
	if err := wait(ctx, 900*time.Millisecond); err != nil {
		check.Fail(err)
		return UpdateResult{}, err
	}
	check.Done("Checked integrated apps")

	updates := []UpdateCandidate{
		{ID: "helium", CurrentVersion: "0.13.2.1", NewVersion: "0.13.3.1"},
		{ID: "obsidian", CurrentVersion: "1.30.0", NewVersion: "1.31.0"},
		{ID: "example", CurrentVersion: "0.1.0", NewVersion: "0.2.0"},
	}

	if req.Confirmation != nil {
		confirmed, err := req.Confirmation.ConfirmUpdates(ctx, updates)
		if err != nil {
			return UpdateResult{}, err
		}
		if !confirmed {
			return UpdateResult{Applied: false, Updates: updates}, nil
		}
	}

	if err := s.downloadUpdates(ctx, activity, updates); err != nil {
		return UpdateResult{}, err
	}

	return UpdateResult{Applied: true, Updates: updates}, nil
}

func (s *DummyService) downloadUpdates(ctx context.Context, activity ActivityReporter, updates []UpdateCandidate) error {
	errCh := make(chan error, len(updates))
	semaphore := make(chan struct{}, 2)

	for _, update := range updates {
		update := update
		go func() {
			errCh <- s.downloadUpdate(ctx, activity, update, semaphore)
		}()
	}

	for range updates {
		if err := <-errCh; err != nil {
			return err
		}
	}

	return nil
}

func (s *DummyService) downloadUpdate(ctx context.Context, activity ActivityReporter, update UpdateCandidate, semaphore chan struct{}) error {
	var waiting ActivityTask

	select {
	case semaphore <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	default:
		waiting = activity.Start(ctx, Activity{Kind: ActivityKindWaiting, AppID: update.ID})
		select {
		case semaphore <- struct{}{}:
			waiting.Done("Ready")
		case <-ctx.Done():
			waiting.Fail(ctx.Err())
			return ctx.Err()
		}
	}
	defer func() { <-semaphore }()

	total := updateDownloadSize(update.ID)
	download := activity.Start(ctx, Activity{
		Kind:      ActivityKindDownloading,
		AppID:     update.ID,
		AssetName: "asset.AppImage",
		Total:     total,
		Unit:      ActivityUnitBytes,
	})

	const mb = 1024 * 1024
	for downloaded := int64(0); downloaded < total; downloaded += 5 * mb {
		if err := wait(ctx, 100*time.Millisecond); err != nil {
			download.Fail(err)
			return err
		}
		download.Advance(5 * mb)
	}
	download.Done("Downloaded " + update.ID)

	return nil
}

func updateDownloadSize(id string) int64 {
	const mb = 1024 * 1024
	switch id {
	case "helium":
		return 140 * mb
	case "obsidian":
		return 130 * mb
	default:
		return 90 * mb
	}
}

func (s *DummyService) List(ctx context.Context, req ListRequest) (ListResult, error) {
	if err := ctx.Err(); err != nil {
		return ListResult{}, err
	}

	return ListResult{
		Items: []ListItem{
			{ID: "helium", Name: "Helium", Version: "0.13.2.1"},
			{ID: "obsidian", Name: "Obsidian", Version: "1.30.0"},
			{ID: "t3-code-alpha", Name: "T3 Code (Alpha)", Version: "0.0.27"},
		},
	}, nil
}

func (s *DummyService) Info(ctx context.Context, req InfoRequest) (InfoResult, error) {
	if err := ctx.Err(); err != nil {
		return InfoResult{}, err
	}

	if looksLikePath(req.Target) {
		name := strings.TrimSuffix(filepath.Base(req.Target), filepath.Ext(req.Target))
		return InfoResult{
			ID:           "",
			Name:         name,
			Version:      "unknown",
			ExecPath:     req.Target,
			Source:       domain.NewLocalSource(req.Target, time.Now()),
			UpdateSource: domain.UpdateSource{},
		}, nil
	}

	return InfoResult{
		ID:           "helium",
		Name:         "Helium",
		Version:      "0.13.2.1",
		ExecPath:     filepath.Join(s.config.AppImageDir, "helium.AppImage"),
		Source:       domain.NewGitHubReleaseSource("imputnet/helium", "v0.13.2.1", "helium.AppImage", "https://github.com/imputnet/helium/releases/latest", 0, time.Now()),
		UpdateSource: domain.NewGitHubUpdateSource("imputnet/helium", false),
	}, nil
}

func looksLikePath(target string) bool {
	return strings.Contains(target, string(filepath.Separator)) || strings.HasSuffix(target, ".AppImage")
}

func (s *DummyService) SelfUpdate(ctx context.Context, req SelfUpdateRequest) (SelfUpdateResult, error) {
	activity := req.Activity
	if activity == nil {
		activity = NoopActivityReporter{}
	}

	const mb = 1024 * 1024
	update := SelfUpdateCandidate{
		CurrentVersion: "0.17.0",
		NewVersion:     "0.18.0",
		AssetName:      "aim-linux-x86_64.AppImage",
		AssetSizeBytes: 20 * mb,
	}

	if req.Confirmation != nil {
		confirmed, err := req.Confirmation.ConfirmSelfUpdate(ctx, update)
		if err != nil {
			return SelfUpdateResult{}, err
		}
		if !confirmed {
			return SelfUpdateResult{Applied: false, Update: update}, nil
		}
	}

	download := activity.Start(ctx, Activity{
		Kind:      ActivityKindDownloading,
		AssetName: update.AssetName,
		Total:     update.AssetSizeBytes,
		Unit:      ActivityUnitBytes,
	})
	for downloaded := int64(0); downloaded < update.AssetSizeBytes; downloaded += 2 * mb {
		if err := wait(ctx, 120*time.Millisecond); err != nil {
			download.Fail(err)
			return SelfUpdateResult{}, err
		}
		download.Advance(2 * mb)
	}
	download.Done("Downloaded " + update.AssetName)

	return SelfUpdateResult{Applied: true, Update: update}, nil
}

func (s *DummyService) Paths(ctx context.Context, req PathsRequest) (PathsResult, error) {
	if err := ctx.Err(); err != nil {
		return PathsResult{}, err
	}

	return PathsResult{
		ConfigFile:  s.config.ConfigFile,
		AppImageDir: s.config.AppImageDir,
		CacheDir:    s.config.CacheDir,
		DesktopDir:  s.config.DesktopDir,
		IconDir:     s.config.IconDir,
	}, nil
}
