# Custom Files for SDKs

This directory contains custom files that are automatically copied to the respective SDKs during generation.

## Directory Structure

```
custom/
├── javascript/          # Custom files for JavaScript/TypeScript SDK
├── python/             # Custom files for Python SDK
└── go/                 # Custom files for Go SDK
```

## How it works

1. Place your custom files in the appropriate SDK subdirectory with the same structure as the target SDK
2. When you run `make generate-<sdk-type>-sdk`, the custom files will be automatically copied to the generated SDK
3. No need to backup/restore - just place files here and they get copied during generation

## Example Usage

### JavaScript SDK

- Create `api/custom/javascript/src/utils/helpers.ts`
- Run `make generate-javascript-sdk`
- File will be copied to `api/javascript/src/utils/helpers.ts`

### Python SDK

- Create `api/custom/python/flexprice/utils/helpers.py`
- Run `make generate-python-sdk`
- File will be copied to `api/python/flexprice/utils/helpers.py`

### Go SDK

- Create `api/custom/go/helpers.go`
- Run `make generate-go-sdk`
- File will be copied to `api/go/helpers.go`

## Important Notes

- Place files here with the same directory structure as the target SDK
- Files in this directory will overwrite any generated files with the same path
- No need to manage backup/restore - just place files here
