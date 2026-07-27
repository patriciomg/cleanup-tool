package categories

type Category string

const (
	Unknown       Category = "unknown"
	LLM           Category = "llm-model"
	Docker        Category = "docker"
	BuildArtifact Category = "build-artifact"
	Dependency    Category = "dependency"
	Media         Category = "media"
	Archive       Category = "archive"
	LogCache      Category = "log-cache"
	Download      Category = "download"
	Document      Category = "document"
	Application   Category = "application"
	GitRepo       Category = "git-repo"
)

type Rule struct {
	Name     string
	Category Category
	MatchFn  func(path string, name string) bool
	Weight   int
}

var extRules = map[string]Category{
	".gguf":        LLM,
	".safetensors": LLM,
	".bin":         LLM,
	".pt":          LLM,
	".pth":         LLM,
	".onnx":        LLM,
	".ckpt":        LLM,
	".ggml":        LLM,
	".h5":          LLM,
	".dmg":         Archive,
	".zip":         Archive,
	".tar":         Archive,
	".gz":          Archive,
	".tgz":         Archive,
	".bz2":         Archive,
	".xz":          Archive,
	".rar":         Archive,
	".7z":          Archive,
	".jpg":         Media,
	".jpeg":        Media,
	".png":         Media,
	".gif":         Media,
	".webp":        Media,
	".heic":        Media,
	".mov":         Media,
	".mp4":         Media,
	".avi":         Media,
	".mkv":         Media,
	".mp3":         Media,
	".aac":         Media,
	".wav":         Media,
	".log":         LogCache,
	".tmp":         LogCache,
	".cache":       LogCache,
}

var nameRules = map[string]Category{
	"node_modules":  Dependency,
	".git":          GitRepo,
	"target":        BuildArtifact,
	".build":        BuildArtifact,
	"build":         BuildArtifact,
	"DerivedData":   BuildArtifact,
	"__pycache__":   BuildArtifact,
	".pytest_cache": BuildArtifact,
	".venv":         Dependency,
	"venv":          Dependency,
	".docker":       Docker,
	"Docker":        Docker,
	".cache":        LogCache,
	"logs":          LogCache,
	".npm":          Dependency,
	".gradle":       Dependency,
	"vendor":        Dependency,
}

func Classify(path, name string) Category {
	if cat, ok := nameRules[name]; ok {
		return cat
	}
	for ext, cat := range extRules {
		if len(name) > len(ext) && name[len(name)-len(ext):] == ext {
			return cat
		}
	}
	if len(path) > 0 && path[len(path)-1] == '/' {
		path = path[:len(path)-1]
	}
	for n, cat := range nameRules {
		if containsPathComponent(path, n) {
			return cat
		}
	}
	return Unknown
}

func containsPathComponent(path, component string) bool {
	if path == component || len(path) > len(component) && (path[len(path)-len(component)-1] == '/' && path[len(path)-len(component):] == component) {
		return true
	}
	for i := 0; i <= len(path)-len(component); i++ {
		if i > 0 && path[i-1] != '/' {
			continue
		}
		if path[i:i+len(component)] == component {
			if i+len(component) == len(path) || path[i+len(component)] == '/' {
				return true
			}
		}
	}
	return false
}
