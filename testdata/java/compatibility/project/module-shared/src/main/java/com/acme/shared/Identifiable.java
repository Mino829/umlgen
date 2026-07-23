package com.acme.shared;

public interface Identifiable<T extends Comparable<T>> {
    T id();
}
