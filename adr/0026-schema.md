# 25. Zarf Schema for 1

Date: 2024-06-07

## Status

Proposed

## Context

Zarf currently does not have explicit schema versions. Any schema changes are embedded into Zarf and can change with new versions. There are several examples of deprecated keys throughout Zarf's lifespan such as:

- `setVariable` deprecated in favor of `setVariables`
- `scripts` deprecated in favor of `actions`
- `required` soon to be deprecated in favor of `optional`.
- `group` deprecated in favor of `flavor`, however these work functionally different and cannot be automatically migrated
- `cosignKeyPath` deprecated in favor of specifying the key path at create time

Zarf has not disabled any deprecated keys thus far. On create the user is always warned when using a deprecated field, however the field still exists in the schema and functions properly. Some of the deprecated keys can be migrated automatically as the old item is a subset or directly related to it's replacement. For example, setVariable is automatically migrated to a single item list of setVariables. This migration occurs on the zarf.yaml fed into the package during package create, however the original field is not deleted from the packaged zarf.yaml because the Zarf binary used in the airgap or delivery environment is not assumed to have the new schema fields that the deprecated key was migrated to.

Creating a v1 schema will allow Zarf to establish a contract with it's user base that features will be supported long term. It will also provide a convenient time for Zarf to drop deprecated features.

## Decision

//TODO how do we introduce experimental features

Zarf will begin having proper schema versions. A top level key, `apiVersion`, will be introduced to allow users to specify the schema. At the release of v1 the only valid user input for `apiVersion` will be v1. Zarf will not allow users to build at the pre v1 version. `zarf package create` will fail if the user has deprecated keys or if `apiVersion` is missing and the user will be instructed to run the new `zarf dev update-schema` command. `zarf dev update-schema` will automatically migrate deprecated fields in the users `zarf.yaml` where possible. It will also add the apiVersion key and set it to v1.

The existing go types which comprise the Zarf schema will be moved to types/alpha and will never change. An updated copy of these types without the deprecated fields will be created in a package types/v1 and any future schema changes will affect these objects. Internally, Zarf will introduce translation functions which will take the alpha schema and return the v1 schema. From that point on, all function signatures that have a struct that is included in the Zarf schema will change from `types.structName` to `v1.structName`.

All deprecated features will cause an error on create. Deprecated features with a direct migration path will still be deployed if the package was created v1, as migrations will add the non deprecated fields. If a feature does not have a direct automatic migration path (cosignKeyPath & groups) the package will fail on deploy. This will happen until the alpha schema is entirely removed from Zarf, which will happen one year after v1 is released.

Any key that exists at the introduction of v1 will last the entirety of that schema lifetime. The features may be deprecated, but will not be removed until the next schema version.

### BDD scenarios
The following are (behavior driven development)[https://en.wikipedia.org/wiki/Behavior-driven_development] scenarios provide context of what Zarf will do in specific situations given the above decisions.

#### v1 create with deprecated keys
- *Given* Zarf version is v1
- *and* the `zarf.yaml` has no apiVersion or deprecated keys
- *when* the user runs `zarf package create`
- *then* they will receive an error and be told to run `zarf dev update-schema` or how to migrate off cosign key paths or how to use flavors over groups depending on the error

#### pre v1 create -> v1 deploy
- *Given*: A package is created with Zarf pre v1
- *and* that package has deprecated keys that can be automatically migrated (required, scripts, & set variables)
- *when* the package is deployed with Zarf v1
- *then* the keys will be automatically migrated & the package will be deployed without error.

#### pre v1 create feature removal -> v1 deploy
- *Given*: A package is created with Zarf pre v1
- *and* that package has deprecated keys that cannot be automatically migrated (groups, cosignKeyPath)
- *when* the package is deployed with Zarf v1
- *then* then deploy of that package will fail and the user will be instructed to update their package

#### v1 create -> pre v1 deploy
- *Given*: A package is created with Zarf v1
- *and* that package uses keys that did not exist in pre v1
- *when* the package is deployed with Zarf pre v1
- *then* Zarf pre 1 will deploy the package without issues. If there is an automatic migration to a previous field that then will take place. If the field is unrecognized by the schema, then the user will be warned they are deploying a package that has features that do not exist in the current version of Zarf.

## Consequences
- As long as the only deprecated features in a package have migration path, and the package was built after the feature was deprecated so migrations were run, Zarf will be successful both creating a package with v1 and deploying with prev1, and creating a package with prev1 and deploying with v1.
- Users of deprecated group or cosignKeyPath might be frustrated if their packages, created prev1, error out on Zarf v1, however this is likely preferable to unexpected behavior occurring in the cluster.
- Users may be frustrated that they have to run `zarf dev update-schema` to edit their `zarf.yaml` to remove the deprecated fields and add `apiVersion`.
- We will have to have two different schema types which will be mostly be duplicate code. However the original type should never change, which mitigates much of the issue.

Below is an example v1 zarf.yaml with, somewhat, reasonable & nonempty values for every key
```yaml
kind: ZarfPackageConfig
apiVersion: v1
metadata:
  name: everything-zarf-package
  description: A zarf package with a non empty value for every
  version: v1.0.0
  url: https://my-package-website.com
  image: https://my-image-url-to-use-in-deprecated-zarf-ui # TODO This field should be deprecated
  uncompressed: true
  architecture: amd64
  yolo: false
  authors: cool-kidz
  documentation: my-package-documentation
  source: https://my-git-server/my-package
  vendor: my-vendor
  aggregateChecksum: shasum # created by Zarf, probably should be moved to the build section
build: # Everything here is created by Zarf not be users
  terminal: my-computer
  user: my-user
  architecture: amd64
  timestamp: 2021-09-01T00:00:00Z
  version: v1.0.0
  migrations:
    - scripts-to-actions
  registryOverrides:
    gcr.io: my-gcr.com
  differential: true
  differentialPackageVersion: "v0.99.9"
  differentialMissing:
    - missing-component
  flavor: cool-flavor
  lastNonBreakingVersion: "v0.99.9"
components:
- name: a-component
  description: Zarf description
  default: false # Austin to check if we remove this
  only:
    localOS: darwin
    cluster:
      architecture: amd64
      distros:
      - ubuntu
    flavor: a-flavor # this will only be used when there are multiple components
  import:
    name: other-component-name
    path: ABCD # Only path or URL will be used, not both
    url: oci://
  manifests:
  - name: manifest
    namespace: manifest-ns
    files:
    - a-file.yaml
    kustomizeAllowAnyDirectory: false
    kustomizations:
    - a-kustomization.yaml
    noWait: false
  charts:
  - name: chart
    namespace: chart-ns
    version: v1.0.0
    url: https://chart-url.com # Can only have one of url or localPath
    localPath: folder
    repoName: repo # We should change these names to make them less confusing https://github.com/defenseunicorns/zarf/issues/2245
    gitPath: charts/podinfo
    releaseName: chart-release
    noWait: true
    valuesFiles:
    - values.yaml
    variables:
      - name: REPLICA_COUNT
        description: "Override the number of pod replicas"
        path: replicaCount
  dataInjections:
  - source: zim-data
    target:
      namespace: my-namespace
      selector: app=my-app
      container: data-loader
      path: /data
    compress: true
  files:
  - source: source-file.txt
    target: target-file.txt
    shasum: shasum
    executable: false
    symlinks:
    - /path/to/symlink
    extractPath: /path/to/extract
  images:
  - podinfo@v1
  repos:
  - https://github.com/defenseunicorns/zarf
  extensions:
    bigbang:
      version: bbVersion
      repo: https://repo1.com/mybbrepo
      valuesFiles:
      - values.yaml
      skipFlux: false
      fluxPatchFiles:
      - flux-patch.yaml
  actions:
    onCreate:
      defaults:
        mute: true
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        shell:
          darwin: sh
          linux: sh
          windows: powershell
      before:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
      after:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
      onSuccess:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
      onFailure:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
    onDeploy:
      defaults:
        mute: true
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        shell:
          darwin: sh
          linux: sh
          windows: powershell
      before:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
      after:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
      onSuccess:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
      onFailure:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
    onRemove:
      defaults:
        mute: true
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        shell:
          darwin: sh
          linux: sh
          windows: powershell
      before:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
      after:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
      onSuccess:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
      onFailure:
      - mute: false
        maxTotalSeconds: 0
        maxRetries: 0
        dir: dir
        env:
        - ENV_VAR=FOO
        cmd: echo hello
        shell:
          darwin: sh
          linux: sh
          windows: powershell
        setVariables:
        - name: VAR
          sensitive: false
          autoIndent: true
          pattern: ".+"
          type: raw
        description: action-description
        wait:
          cluster: # Only one of cluster / network can be used
            kind: pod
            name: my-pod
            namespace: pod-ns
            condition: ready
          network:
            protocol: http
            address: github.com
            code: 200
constants:
- name: CONSTANT
  value: constant-value
  description: constant-value
  autoIndent: false
  pattern: ".+"
variables:
- name: VAR
  sensitive: false
  autoIndent: true
  pattern: ".+"
  type: raw
  description: var
  default: whatever
  prompt: false
```
