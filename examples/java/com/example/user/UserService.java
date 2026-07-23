package com.example.user;

public class UserService {
    private final UserRepository repository;

    public UserService(UserRepository repository) {
        this.repository = repository;
    }

    public User findById(long id) {
        return repository.findById(id);
    }
}
