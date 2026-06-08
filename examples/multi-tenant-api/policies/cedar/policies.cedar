
permit (
    principal,
    action,
    resource
) when {
    context.level == "tenant" &&
    context.isMember == true &&
    context.exp > 0
};

permit (
    principal,
    action,
    resource
) when {
    context.level == "tenant" &&
    context.isMember == true
};

permit (
    principal,
    action,
    resource
) when {
    context.level == "resource" &&
    context.isOwned == true &&
    context.isMember == true &&
    context.exp > 0
};

permit (
    principal,
    action,
    resource
) when {
    context.level == "resource" &&
    context.isOwned == true &&
    context.isMember == true
};
