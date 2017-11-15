# Release

## Bump

Run the `bump-release` script.

```
./build/bump-release vX.Y.Z
```

## Push

Create a PR for the release.

```
git push REMOTE feature:feature
```

## Release

Merge the release PR and create a Github release. Be sure the tag appears on Github.

```
git push origin vX.Y.Z
```

## Publish

Manually trigger the Jenkins [pipeline](https://jenkins-os.prod.coreos.systems/job/cluo/job/release/) with "Build with Parameters". Specify the tag vX.Y.Z to release.

