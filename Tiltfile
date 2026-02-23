# BuilderHub build-operator - rebuilds on Go/file changes
# Install CRDs then apply builder size templates (builder-small, builder-medium, builder-large, builder-xlarge)
local_resource(
    'build-operator',
    cmd='make install && kubectl apply -f config/samples/builder-templates.yaml',
    serve_cmd='make run',
    deps=['cmd', 'internal', 'api', 'config/samples/builder-templates.yaml'],
    ignore=['bin', 'config/crd', 'config/rbac', 'config/webhook'],
    allow_parallel=True,
)
