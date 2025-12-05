# Docker Compose V2 Migration

## Overview

The GitHub Actions workflow has been updated to use **Docker Compose V2** syntax.

## What Changed

### Old Command (V1)
```bash
docker-compose up -d
docker-compose build
docker-compose logs
```

### New Command (V2)
```bash
docker compose up -d
docker compose build
docker compose logs
```

**Key Difference:** Hyphen (`-`) replaced with space (` `)

## Why This Change?

1. **GitHub Actions Default:** GitHub Actions runners now use Docker Compose V2 by default
2. **Docker Official:** Docker Compose V2 is now the official version integrated into Docker CLI
3. **Better Performance:** V2 is written in Go and is faster than V1 (Python-based)
4. **Active Development:** V2 is actively maintained, V1 is deprecated

## Compatibility

### Docker Compose V2 Features

- ✅ Fully backward compatible with V1 compose files
- ✅ Same commands, just different invocation
- ✅ All `docker-compose.yml` files work without changes
- ✅ Improved performance and stability

### Checking Your Version

```bash
# Check if V2 is available
docker compose version

# Output example:
# Docker Compose version v2.23.0
```

### If V2 Not Installed

```bash
# Install Docker Compose V2 (Linux)
sudo apt-get update
sudo apt-get install docker-compose-plugin

# Install Docker Compose V2 (Mac)
# Already included in Docker Desktop

# Install Docker Compose V2 (Windows)
# Already included in Docker Desktop
```

## Local Development

### Option 1: Use V2 (Recommended)

```bash
cd test
docker compose up -d
docker compose run --rm test-client /test-client
docker compose down -v
```

### Option 2: Use Makefile (Abstracted)

The Makefile has been updated to use V2:

```bash
cd test
make up      # Uses docker compose
make test    # Uses docker compose
make down    # Uses docker compose
```

### Option 3: Keep Using V1 Locally

If you still have `docker-compose` V1 installed:

```bash
cd test
docker-compose up -d    # Still works locally
```

**Note:** GitHub Actions will always use V2.

## Updated Files

All references updated from `docker-compose` to `docker compose`:

- ✅ `.github/workflows/gateway-tests.yml` - GitHub Actions workflow
- ✅ `.github/workflows/README.md` - Workflow documentation
- ✅ `.github/TESTING.md` - Testing guide
- ✅ `CI_CD_SUMMARY.md` - CI/CD summary
- ✅ Documentation examples

## Migration Guide

### For Contributors

No action needed if using:
- Docker Desktop (Mac/Windows) - V2 included
- Recent Linux Docker installation - V2 included
- Makefile commands - Abstracted

### For CI/CD Pipelines

Update any scripts that use `docker-compose`:

```bash
# Before
docker-compose up -d
docker-compose run test

# After
docker compose up -d
docker compose run test
```

### For Scripts

Replace hyphen with space:

```bash
# Automated replacement
sed -i 's/docker-compose/docker compose/g' your-script.sh
```

## Troubleshooting

### "command not found: docker compose"

**Solution 1:** Install Docker Compose V2
```bash
# Linux
sudo apt-get install docker-compose-plugin

# Mac/Windows
Update Docker Desktop to latest version
```

**Solution 2:** Use V1 locally (temporary)
```bash
docker-compose up -d  # Keep using V1
```

**Solution 3:** Create alias (temporary workaround)
```bash
# Add to ~/.bashrc or ~/.zshrc
alias docker-compose='docker compose'
```

### GitHub Actions Still Failing

Check these:

1. ✅ Workflow uses `docker compose` (with space)
2. ✅ No custom scripts use `docker-compose` (with hyphen)
3. ✅ Docker Buildx action is at v3 or later
4. ✅ Using `ubuntu-latest` runner image

### Makefile Not Working

Update Makefile to use V2:

```makefile
# Before
up:
	docker-compose up -d

# After
up:
	docker compose up -d
```

## Benefits of V2

1. **Performance:** 2-3x faster than V1
2. **Integration:** Built into Docker CLI
3. **Stability:** Fewer bugs, better error messages
4. **Features:** New features and improvements
5. **Support:** Active development and updates

## Backward Compatibility

Docker Compose V2 is **100% backward compatible** with:

- ✅ `docker-compose.yml` files (no changes needed)
- ✅ Compose file versions 2.x and 3.x
- ✅ All commands and flags
- ✅ Environment variables
- ✅ Override files

## Version Comparison

| Feature | V1 (docker-compose) | V2 (docker compose) |
|---------|---------------------|---------------------|
| Command | `docker-compose` | `docker compose` |
| Language | Python | Go |
| Performance | Baseline | 2-3x faster |
| Integration | Standalone | Docker CLI plugin |
| Support | Deprecated | Active |
| Installation | pip/binary | Docker plugin |

## Testing the Change

### Local Test

```bash
cd test

# Test with V2
docker compose up -d
docker compose ps
docker compose logs
docker compose down

# If successful, V2 is working
```

### GitHub Actions Test

```bash
# Trigger workflow
git push origin master

# Or manually
gh workflow run "HTTP Gateway Tests"

# Check results
gh run list --workflow="HTTP Gateway Tests"
```

## References

- [Docker Compose V2 Announcement](https://www.docker.com/blog/announcing-compose-v2-general-availability/)
- [Docker Compose V2 Documentation](https://docs.docker.com/compose/compose-v2/)
- [Migration Guide](https://docs.docker.com/compose/migrate/)

## Summary

- ✅ **Command changed:** `docker-compose` → `docker compose`
- ✅ **Compose files unchanged:** No modifications needed
- ✅ **GitHub Actions updated:** All workflows use V2
- ✅ **Local development:** Works with V1 or V2
- ✅ **Makefile abstracted:** Uses V2 commands
- ✅ **Fully compatible:** 100% backward compatible

The change is **non-breaking** for local development and **required** for GitHub Actions.
