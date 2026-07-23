package com.acme.sales;

public class Order {
    public record LineItem(User product, int quantity) {
    }
}
