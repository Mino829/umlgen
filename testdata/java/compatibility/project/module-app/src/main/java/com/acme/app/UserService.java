package com.acme.app;

import com.acme.sales.*;
import com.acme.support.User;
import java.util.List;
import java.util.Optional;

public class UserService {
    private final User owner;
    private final com.acme.sales.User salesUser;
    private final List<Order.LineItem> lineItems;
    private final ExternalAuditClient auditClient;

    public UserService(
        User owner,
        com.acme.sales.User salesUser,
        List<Order.LineItem> lineItems,
        ExternalAuditClient auditClient
    ) {
        this.owner = owner;
        this.salesUser = salesUser;
        this.lineItems = lineItems;
        this.auditClient = auditClient;
    }

    public Optional<com.acme.sales.User> findSalesUser() {
        return Optional.of(salesUser);
    }

    public List<? extends com.acme.sales.User> findAll() {
        return List.of(salesUser);
    }
}
