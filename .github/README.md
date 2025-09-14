# GitHub Actions Setup

This repository includes GitHub Actions workflows for automated building, testing, and deployment.

## Workflows

### 1. `docker.yml` - Docker Build and Push
- **Triggers**: Push to main/master branches and tags
- **Actions**:
  - Builds multi-platform Docker image (linux/amd64, linux/arm64)
  - Pushes to Docker Hub
  - Builds installer YAML using `make build-installer`
  - Uploads installer as artifact

### 2. `build.yml` - Full CI/CD Pipeline
- **Triggers**: Push to main/master branches, tags, and pull requests
- **Actions**:
  - Runs tests and linting
  - Builds and pushes Docker image
  - Builds installer YAML
  - Creates GitHub releases for tags

## Required Secrets

To use these workflows, you need to set up the following secrets in your GitHub repository:

### Docker Hub Secrets
1. Go to your GitHub repository
2. Navigate to Settings → Secrets and variables → Actions
3. Add the following secrets:

| Secret Name | Description | Example |
|-------------|-------------|---------|
| `DOCKERHUB_USERNAME` | Your Docker Hub username | `myusername` |
| `DOCKERHUB_TOKEN` | Your Docker Hub access token | `dckr_pat_...` |

### How to Get Docker Hub Token
1. Go to [Docker Hub](https://hub.docker.com/)
2. Sign in to your account
3. Go to Account Settings → Security
4. Click "New Access Token"
5. Give it a name (e.g., "GitHub Actions")
6. Copy the token and add it as `DOCKERHUB_TOKEN` secret

## Workflow Features

### Multi-Platform Builds
- Builds for both `linux/amd64` and `linux/arm64`
- Uses Docker Buildx for cross-platform support
- Caches layers for faster builds

### Automatic Tagging
- **Branches**: `latest` for main/master, branch name for others
- **Tags**: Semantic versioning (e.g., `v1.0.0`, `v1.0`, `v1`)
- **PRs**: Branch name with PR number

### Helm Chart Packaging
- Automatically packages the Helm chart using `helm package`
- Updates the image repository and tag in `values.yaml`
- Uploads packaged chart (.tgz) as artifact for releases

### GitHub Releases
- Automatically creates releases for version tags
- Includes installation instructions
- Attaches the installer YAML file

## Usage Examples

### Building Locally
```bash
# Build the Docker image
make docker-build

# Build the installer
make build-installer

# Build and push to Docker Hub
make docker-buildx IMG=yourusername/dbchan:latest
```

### Installing from GitHub Releases
```bash
# Download and install the Helm chart
curl -LO https://github.com/yourusername/dbchan/releases/download/v1.0.0/dbchan-0.1.0.tgz
helm install dbchan dbchan-0.1.0.tgz
```

### Installing from Docker Hub
```bash
# Install using the latest image with Helm
helm install dbchan ./charts/dbchan --set image.repository=yourusername/dbchan --set image.tag=latest
```

## Customization

### Changing the Image Name
Update the `IMAGE_NAME` environment variable in the workflow files:

```yaml
env:
  REGISTRY: docker.io
  IMAGE_NAME: yourusername/dbchan  # Change this
```

### Adding More Platforms
Modify the `platforms` parameter in the Docker build step:

```yaml
platforms: linux/amd64,linux/arm64,linux/s390x,linux/ppc64le
```

### Changing Go Version
Update the Go version in the workflow:

```yaml
- name: Set up Go
  uses: actions/setup-go@v4
  with:
    go-version: '1.21'  # Change this
```

## Troubleshooting

### Common Issues

1. **Docker Hub Authentication Failed**
   - Check that `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` secrets are set correctly
   - Verify the token has the correct permissions

2. **Build Fails on Multi-Platform**
   - Ensure Docker Buildx is properly set up
   - Check that the base image supports the target platforms

3. **Installer Build Fails**
   - Verify that `make build-installer` works locally
   - Check that all required tools are installed

4. **Release Creation Fails**
   - Ensure `GITHUB_TOKEN` has the correct permissions
   - Check that the tag format is correct (e.g., `v1.0.0`)

### Debugging

To debug workflow issues:
1. Check the Actions tab in your GitHub repository
2. Click on the failed workflow run
3. Expand the failed step to see the error details
4. Check the logs for specific error messages

### Local Testing

Before pushing, test locally:

```bash
# Test the build process
make docker-build
make build-installer

# Test with different image names
export IMG=yourusername/dbchan:test
make build-installer
```
