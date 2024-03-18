# -*- mode: Python -*-

kubectl_cmd = "kubectl"

# verify kubectl command exists
if str(local("command -v " + kubectl_cmd + " || true", quiet = True)) == "":
    fail("Required command '" + kubectl_cmd + "' not found in PATH")

# set defaults
settings = {
    "create_secrets": {
        "enable": True,
        "token": os.getenv("GITHUB_TOKEN", ""),
        "email": os.getenv("GITHUB_EMAIL", ""),
        "user": os.getenv("GITHUB_USER", ""),
    },
    "verification_keys": {},
}

# global settings
tilt_file = "./tilt-settings.yaml" if os.path.exists("./tilt-settings.yaml") else "./tilt-settings.json"
settings.update(read_yaml(
    tilt_file,
    default = {},
))
load('ext://secret', 'secret_yaml_registry', 'secret_from_dict', 'secret_create_generic')


def create_secrets():
    opts = settings.get("create_secrets")
    if not opts.get("enable"):
        return

    k8s_yaml(secret_from_dict("creds", "ocm-system", inputs = {
        'username' : opts.get('user'),
        'password' : opts.get('token'),
    }), allow_duplicates = True)


def create_verification_keys():
    keys = settings.get("verification_keys")
    if not keys:
        return

    for key, value in keys.items():
        secret_create_generic(key, 'ocm-system', from_file=value)


# Use kustomize to build the install yaml files
install = kustomize('config/default')

# Update the root security group. Tilt requires root access to update the
# running process.
objects = decode_yaml_stream(install)
for o in objects:
    if o.get('kind') == 'Deployment' and o.get('metadata').get('name') == 'replication-controller':
        o['spec']['template']['spec']['containers'][0]['securityContext']['runAsNonRoot'] = False
        break

updated_install = encode_yaml_stream(objects)

# Apply the updated yaml to the cluster.
# Allow duplicates so the e2e test can include this tilt file with other tilt files
# setting up the same namespace.
k8s_yaml(updated_install, allow_duplicates = True)

# Create Secrets
create_secrets()
create_verification_keys()

load('ext://restart_process', 'docker_build_with_restart')

# enable hot reloading by doing the following:
# - locally build the whole project
# - create a docker imagine using tilt's hot-swap wrapper
# - push that container to the local tilt registry
# Once done, rebuilding now should be a lot faster since only the relevant
# binary is rebuilt and the hot swat wrapper takes care of the rest.
local_resource(
    'replication-controller-binary',
    'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/manager ./',
    deps = [
        "main.go",
        "go.mod",
        "go.sum",
        "api",
        "controllers",
        "pkg",
    ],
)

# Build the docker image for our controller. We use a specific Dockerfile
# since tilt can't run on a scratch container.
# `only` here is important, otherwise, the container will get updated
# on _any_ file change. We only want to monitor the binary.
# If debugging is enabled, we switch to a different docker file using
# the delve port.
entrypoint = ['/manager']
dockerfile = 'tilt.dockerfile'
docker_build_with_restart(
    'ghcr.io/open-component-model/replication-controller',
    '.',
    dockerfile = dockerfile,
    entrypoint = entrypoint,
    only=[
      './bin',
    ],
    live_update = [
        sync('./bin/manager', '/manager'),
    ],
)
