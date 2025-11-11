package giturl

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    *ParsedURL
		wantErr bool
	}{
		{
			name: "HTTPS with .git",
			url:  "https://github.com/user/repo.git",
			want: &ParsedURL{Host: "github.com", Path: "user/repo", Org: "user", Repo: "repo"},
		},
		{
			name: "HTTPS without .git",
			url:  "https://github.com/user/repo",
			want: &ParsedURL{Host: "github.com", Path: "user/repo", Org: "user", Repo: "repo"},
		},
		{
			name: "SSH git@ format",
			url:  "git@github.com:user/repo.git",
			want: &ParsedURL{Host: "github.com", Path: "user/repo", Org: "user", Repo: "repo"},
		},
		{
			name: "SSH URL format",
			url:  "ssh://git@github.com/user/repo.git",
			want: &ParsedURL{Host: "github.com", Path: "user/repo", Org: "user", Repo: "repo"},
		},
		{
			name: "HTTP",
			url:  "http://gitlab.com/org/project.git",
			want: &ParsedURL{Host: "gitlab.com", Path: "org/project", Org: "org", Repo: "project"},
		},
		{
			name:    "Empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "Invalid format",
			url:     "not-a-git-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && got != nil {
				if got.Host != tt.want.Host || got.Path != tt.want.Path || got.Org != tt.want.Org || got.Repo != tt.want.Repo {
					t.Errorf("Parse() = %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://github.com/user/repo.git", "repo"},
		{"git@github.com:user/myproject.git", "myproject"},
		{"https://github.com/user/repo", "repo"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := ExtractRepoName(tt.url)
			if got != tt.want {
				t.Errorf("ExtractRepoName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSSH(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"git@github.com:user/repo.git", true},
		{"ssh://git@github.com/user/repo.git", true},
		{"https://github.com/user/repo.git", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := IsSSH(tt.url)
			if got != tt.want {
				t.Errorf("IsSSH() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToSSH(t *testing.T) {
	url := "https://github.com/user/repo.git"
	want := "git@github.com:user/repo.git"
	got, err := ToSSH(url)
	if err != nil {
		t.Fatalf("ToSSH() error = %v", err)
	}
	if got != want {
		t.Errorf("ToSSH() = %v, want %v", got, want)
	}
}

func TestToHTTPS(t *testing.T) {
	url := "git@github.com:user/repo.git"
	want := "https://github.com/user/repo.git"
	got, err := ToHTTPS(url)
	if err != nil {
		t.Fatalf("ToHTTPS() error = %v", err)
	}
	if got != want {
		t.Errorf("ToHTTPS() = %v, want %v", got, want)
	}
}
