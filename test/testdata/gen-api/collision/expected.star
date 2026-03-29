# Generated from spec.yaml by scampi gen api (test)
#
# Collision Test 1.0
#
# This file was mechanically generated from an OpenAPI specification.
# It is provided as-is with no warranty. Scampi's license does not
# apply to generated output. If the source specification carries its
# own license terms, those terms govern this file.
#
# Usage: load("spec.api.star", ...)

# {Username}
# -----------------------------------------------------------------------------

# PUT /users/{username} — Update user
def update_user(
        username,
        email = None,
        body_username = None):
    body = {
    }
    if email != None:
        body["email"] = email
    if body_username != None:
        body["username"] = body_username
    return rest.request(
        method = "PUT",
        path = "/users/{username}",
        body = rest.body.json(body),
    )
