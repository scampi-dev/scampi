# Generated from spec.yaml by scampi gen api (test)
#
# Test API 1.0
#
# This file was mechanically generated from an OpenAPI specification.
# It is provided as-is with no warranty. Scampi's license does not
# apply to generated output. If the source specification carries its
# own license terms, those terms govern this file.
#
# Usage: load("spec.api.star", ...)

# Items
# -----------------------------------------------------------------------------

# GET /items — List items
def list_items(check = None):
    return rest.request(
        method = "GET",
        path = "/items",
        check = check,
    )

# POST /items — Create an item
def create_item(name, count = None):
    body = {
        "name": name,
    }
    if count != None:
        body["count"] = count
    return rest.request(
        method = "POST",
        path = "/items",
        body = rest.body.json(body),
    )


# {Id}
# -----------------------------------------------------------------------------

# PUT /items/{id} — Update an item
def update_item(id, name):
    body = {
        "name": name,
    }
    return rest.request(
        method = "PUT",
        path = "/items/{id}",
        body = rest.body.json(body),
    )

# DELETE /items/{id} — Delete an item
def delete_item(id):
    return rest.request(
        method = "DELETE",
        path = "/items/{id}",
    )
