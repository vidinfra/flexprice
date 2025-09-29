# JavaScript SDK Custom Files

Place your custom files for the JavaScript/TypeScript SDK here.

## Directory Structure

```
javascript/
├── src/
│   ├── utils/           # Custom utilities and helpers
│   ├── extensions/      # Extended API methods
│   └── types/          # Additional type definitions
├── examples/           # Custom examples
├── docs/              # Custom documentation
└── config/            # Custom configuration files
```

## Example Usage

To add a custom utility file:

1. Create `api/custom/javascript/src/utils/helpers.ts`
2. Run `make generate-javascript-sdk`
3. The file will be copied to `api/javascript/src/utils/helpers.ts`
