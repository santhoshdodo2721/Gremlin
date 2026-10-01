package dockerapi

import "testing"

func TestApplicationContainersExcludeOutsideProjects(t *testing.T) {
	directory := "/workspace/resilentops/aut"
	containers := []Container{
		{Names: []string{"/aut-db"}, Labels: map[string]string{"com.docker.compose.project.working_dir": directory}},
		{Names: []string{"/aut-api"}, Labels: map[string]string{"com.docker.compose.project.working_dir": directory}},
		{Names: []string{"/grafana"}, Labels: map[string]string{"com.docker.compose.project.working_dir": "/workspace/resilentops/monitoring"}},
		{Names: []string{"/aut-api"}, Labels: map[string]string{"com.docker.compose.project.working_dir": "/other/aut", "com.docker.compose.project": "aut"}},
		{Names: []string{"/aut-api"}},
		{Names: []string{"/outside"}, Labels: map[string]string{"com.docker.compose.project.working_dir": "/other/aut", "com.docker.compose.project.config_files": directory + "/docker-compose.yml"}},
	}
	got := ApplicationContainers(containers, directory)
	if len(got) != 2 || got[0].DisplayName() != "aut-api" || got[1].DisplayName() != "aut-db" {
		t.Fatalf("scope leaked or order incorrect: %+v", got)
	}
	if len(ApplicationContainers(containers, "")) != 0 {
		t.Fatal("empty scope must not expose any containers")
	}
}
func TestApplicationConfigFileFallback(t *testing.T) {
	got := ApplicationContainers([]Container{{Names: []string{"/api"}, Labels: map[string]string{"com.docker.compose.project.config_files": "/workspace/aut/compose.yml,/workspace/aut/override.yml"}}}, "/workspace/aut")
	if len(got) != 1 {
		t.Fatal("Compose config ownership was not recognized")
	}
}
func TestMatchesContainerIdentity(t *testing.T) {
	c := Container{ID: "123456789abc0123456789", Names: []string{"/aut-api"}}
	for _, target := range []string{c.ID, "123456789abc", "aut-api", "/aut-api"} {
		if !c.Matches(target) {
			t.Errorf("did not match %s", target)
		}
	}
	for _, target := range []string{"123", "outside", ""} {
		if c.Matches(target) {
			t.Errorf("matched ambiguous target %q", target)
		}
	}
}
