package com.acme.sales;

import com.acme.shared.DomainType;
import com.acme.shared.Identifiable;

@DomainType("sales")
public final class User implements Identifiable<String> {
    private final String id;

    public User(String id) {
        this.id = id;
    }

    public String id() {
        return id;
    }
}
