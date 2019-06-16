# Github Pull Request Creator

## Table of Contents

* [About](#about)
* [Requirement](#requirement)
* [Build](#build)
* [Usage](#usage)
* [Contributing](#contributing)
* [License](#license)

## About

This command line tool can create new pull request easily from simple config file. Config file can set **base** branch and **head** branch. This tool compare `base` branch with `head` branch. If `head` branch forwarded than `base` branch, this tool create new pull request automatically. Therefore this tool makes your development more efficiently.

## Requirement

* Go v1.12 or higher

## Build

```shell
$ GO111MODULE=on go build -o bin/github-pr-creator
```

## Usage

### 1. Create config file

At first, you create `app.config.json` config file, like this.

```json
[
  {
    "owner": "Your owner name",
    "repo": "Your repository name",
    "head": "Head branch name",
    "base": "Base branch name"
  }
]
```

### 2. Set environ variable

Next, you must set `GITHUB_ACCESS_TOKEN` environ variable.

```shell
export GITHUB_ACCESS_TOKEN=<Your github token>
```

### Run

```shell
$ ./bin/github-pr-creator
```

#### Command line options

```shell
$ ./bin/github-pr-creator --help
Usage: github-pr-creator [--dry-run]

Options:
  --dry-run              dry run mode
  --help, -h             display this help and exit
```

## Contributing

Open an [issue](https://github.com/naoki-sawada/github-pr-creator/issues/new) or submit [PRs](https://github.com/naoki-sawada/github-pr-creator/pulls).

## License

[MIT](LICENSE).