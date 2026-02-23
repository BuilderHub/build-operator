# BuilderHub build-operator - rebuilds on Go/file changes
local_resource(
    'build-operator',
    cmd='make install',
    serve_cmd='make run',
    deps=['cmd', 'internal', 'api'],
    ignore=['bin', 'config/crd', 'config/rbac', 'config/webhook'],
    allow_parallel=True,
)
