from flask import Flask, request
from mitmproxy import ctx

# must match code in mitmproxy.go
MITM_DOMAIN_NAME = "mitm.code"
app = Flask("mitmoptset")

prev_options = {
    "options": {},
}


# Set options on mitmproxy. See https://docs.mitmproxy.org/stable/concepts-options/
# This is intended to be used exclusively for our addons in this package, but nothing
# stops tests from enabling/disabling/tweaking other mitmproxy options.
# POST /options/lock
# {
#   "options": {
#     "body_size_limit": "3m",
#   }
# }
# HTTP/1.1 200 OK
# {}
@app.route("/options/lock", methods=["POST"])
def lock_options():
    if prev_options.get("options") is not None:
        unlock_options()
    body = request.json
    options = body.get("options", {})
    for k, v in ctx.options.items():
        if k in options:
            prev_options["options"][k] = v.current()
    print(f"locking options {options}")
    ctx.options.update(**options)
    return {}


def unlock_options():
    print(f"unlocking options back to {prev_options['options']}")
    ctx.options.update(**prev_options["options"])
    prev_options["options"] = {}
