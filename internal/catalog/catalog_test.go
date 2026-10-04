package catalog

import (
	"strings"
	"testing"

	"yapl/internal/paths"
	"yapl/internal/version"
)

func TestDefault(t *testing.T) {
	g := Default()

	t.Run("given the catalog then its preferred proton is defined", func(t *testing.T) {
		if _, ok := g.ProtonVersions[g.Defaults.Proton]; !ok || g.Defaults.Proton == "" {
			t.Fatalf("defaults.proton %q not in proton_versions", g.Defaults.Proton)
		}
	})

	t.Run("given the catalog then its preferred runtime is defined and checks for updates", func(t *testing.T) {
		rt, ok := g.RuntimeVersions[g.Defaults.Runtime]
		if !ok || !rt.CheckForUpdates {
			t.Fatalf("runtime %q missing or not update-checked: %+v", g.Defaults.Runtime, rt)
		}
	})

	t.Run("given the catalog then its preferred dxvk and vkd3d are defined", func(t *testing.T) {
		if _, ok := g.DependencyVersions[paths.KindDXVK][g.Defaults.DXVK]; !ok {
			t.Fatalf("dxvk %q not defined", g.Defaults.DXVK)
		}
		if _, ok := g.DependencyVersions[paths.KindVKD3D][g.Defaults.VKD3D]; !ok {
			t.Fatalf("vkd3d %q not defined", g.Defaults.VKD3D)
		}
	})

	t.Run("given every pinned component then it downloads over https as a supported archive", func(t *testing.T) {
		urls := []string{}
		for _, v := range g.ProtonVersions {
			urls = append(urls, v.URL)
		}
		for _, v := range g.RuntimeVersions {
			urls = append(urls, v.URL)
		}
		for _, kind := range g.DependencyVersions {
			for _, v := range kind {
				urls = append(urls, v.URL)
			}
		}
		for _, u := range urls {
			if !strings.HasPrefix(u, "https://") {
				t.Errorf("not https: %q", u)
			}
			if !strings.HasSuffix(u, ".tar.gz") && !strings.HasSuffix(u, ".tar.xz") && !strings.HasSuffix(u, ".tar.zst") {
				t.Errorf("unsupported archive: %q", u)
			}
		}
	})

	t.Run("given two calls then each returns an independent copy", func(t *testing.T) {
		a := Default()
		a.ProtonVersions["mutated"] = a.ProtonVersions[a.Defaults.Proton]
		if _, leaked := Default().ProtonVersions["mutated"]; leaked {
			t.Fatal("Default shares maps between calls")
		}
	})
}

func TestModule(t *testing.T) {
	t.Run("given the catalog module then it reports a valid version and no commands", func(t *testing.T) {
		m := Module{}
		if m.Name() != "catalog" || !version.Valid(m.Version()) || len(m.Commands()) != 0 {
			t.Fatalf("module = %s %s %d commands", m.Name(), m.Version(), len(m.Commands()))
		}
	})
}

func TestDefaultRecordsCatalogVersion(t *testing.T) {
	t.Run("given the catalog then the runner config it produces records the catalog version", func(t *testing.T) {
		if Default().CatalogVersion != Version {
			t.Fatalf("catalog_version = %q", Default().CatalogVersion)
		}
	})
}
