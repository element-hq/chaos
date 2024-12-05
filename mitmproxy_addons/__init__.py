from controller import MITM_DOMAIN_NAME, app
from mitmproxy.addons import asgiapp

from callback import Callback

addons = [
    asgiapp.WSGIApp(
        app, MITM_DOMAIN_NAME, 80
    ),  # requests to this host will be routed to the flask app
    Callback(),
]
