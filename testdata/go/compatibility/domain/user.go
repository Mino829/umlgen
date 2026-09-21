package domain

type ID string

type Profile struct {
	Bio string
}

type Tag struct {
	Name string
}

type User struct {
	ID      ID
	Name    string
	Profile *Profile
	Tags    []Tag
}

func (u *User) DisplayName(prefix string) string {
	return prefix + u.Name
}
