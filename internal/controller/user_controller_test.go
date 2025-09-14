/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/base64"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbv1 "github.com/fatiudeen/dbchan/api/v1"
)

var _ = Describe("User Controller", func() {
	var (
		ctx                context.Context
		user               *dbv1.User
		database           *dbv1.Database
		datastore          *dbv1.Datastore
		secret             *corev1.Secret
		userName           string
		databaseName       string
		datastoreName      string
		secretName         string
		namespace          string
		typeNamespacedName types.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
		userName = "test-user"
		databaseName = "test-database"
		datastoreName = "test-datastore"
		secretName = "test-secret"
		typeNamespacedName = types.NamespacedName{
			Name:      userName,
			Namespace: namespace,
		}
	})

	AfterEach(func() {
		// Cleanup user
		if user != nil {
			Expect(k8sClient.Delete(ctx, user)).To(Succeed())
		}
		// Cleanup database
		if database != nil {
			Expect(k8sClient.Delete(ctx, database)).To(Succeed())
		}
		// Cleanup datastore
		if datastore != nil {
			Expect(k8sClient.Delete(ctx, datastore)).To(Succeed())
		}
		// Cleanup secret
		if secret != nil {
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		}
	})

	Context("When creating a User with ready datastore and database", func() {
		BeforeEach(func() {
			By("Creating a secret with database credentials")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"sslmode":  []byte("disable"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating a ready Datastore")
			datastore = &dbv1.Datastore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      datastoreName,
					Namespace: namespace,
				},
				Spec: dbv1.DatastoreSpec{
					DatastoreType: "postgres",
					SecretRef: dbv1.DatastoreSecretRef{
						Name: secretName,
					},
				},
				Status: dbv1.DatastoreStatus{
					Phase: "Ready",
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())

			By("Creating a ready Database")
			database = &dbv1.Database{
				ObjectMeta: metav1.ObjectMeta{
					Name:      databaseName,
					Namespace: namespace,
				},
				Spec: dbv1.DatabaseSpec{
					DatastoreRef: corev1.LocalObjectReference{
						Name: datastoreName,
					},
				},
				Status: dbv1.DatabaseStatus{
					Phase: "Ready",
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, database)).To(Succeed())

			By("Creating a User resource")
			user = &dbv1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name:      userName,
					Namespace: namespace,
				},
				Spec: dbv1.UserSpec{
					Username: "app_user",
					Password: base64.StdEncoding.EncodeToString([]byte("password123")),
					DatastoreRef: corev1.LocalObjectReference{
						Name: datastoreName,
					},
					DatabaseRef: &corev1.LocalObjectReference{
						Name: databaseName,
					},
					Roles:      []string{"readwrite"},
					Privileges: []string{"SELECT", "INSERT", "UPDATE"},
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())
		})

		It("should add finalizer and update status to Creating", func() {
			By("Reconciling the User")
			controllerReconciler := &UserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the finalizer is added")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, user)
				return err == nil && len(user.Finalizers) > 0
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, user)
				return err == nil && user.Status.Phase == "Creating"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})

		It("should handle user deletion with finalizer", func() {
			By("Adding finalizer manually")
			user.Finalizers = append(user.Finalizers, userFinalizer)
			Expect(k8sClient.Update(ctx, user)).To(Succeed())

			By("Deleting the User")
			Expect(k8sClient.Delete(ctx, user)).To(Succeed())

			By("Reconciling the User")
			controllerReconciler := &UserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the finalizer is removed")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, user)
				return err != nil && errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When creating a User without database reference", func() {
		BeforeEach(func() {
			By("Creating a secret with database credentials")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"sslmode":  []byte("disable"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating a ready Datastore")
			datastore = &dbv1.Datastore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      datastoreName,
					Namespace: namespace,
				},
				Spec: dbv1.DatastoreSpec{
					DatastoreType: "postgres",
					SecretRef: dbv1.DatastoreSecretRef{
						Name: secretName,
					},
				},
				Status: dbv1.DatastoreStatus{
					Phase: "Ready",
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())

			By("Creating a User resource without database reference")
			user = &dbv1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name:      userName,
					Namespace: namespace,
				},
				Spec: dbv1.UserSpec{
					Username: "global_user",
					Password: base64.StdEncoding.EncodeToString([]byte("password123")),
					DatastoreRef: corev1.LocalObjectReference{
						Name: datastoreName,
					},
					// No DatabaseRef - global user
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())
		})

		It("should create global user successfully", func() {
			By("Reconciling the User")
			controllerReconciler := &UserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, user)
				return err == nil && user.Status.Phase == "Creating"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When creating a User with non-ready datastore", func() {
		BeforeEach(func() {
			By("Creating a secret with database credentials")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
					"host":     []byte("localhost"),
					"port":     []byte("5432"),
					"sslmode":  []byte("disable"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating a non-ready Datastore")
			datastore = &dbv1.Datastore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      datastoreName,
					Namespace: namespace,
				},
				Spec: dbv1.DatastoreSpec{
					DatastoreType: "postgres",
					SecretRef: dbv1.DatastoreSecretRef{
						Name: secretName,
					},
				},
				Status: dbv1.DatastoreStatus{
					Phase: "Connecting",
					Ready: false,
				},
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())

			By("Creating a User resource")
			user = &dbv1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name:      userName,
					Namespace: namespace,
				},
				Spec: dbv1.UserSpec{
					Username: "app_user",
					Password: base64.StdEncoding.EncodeToString([]byte("password123")),
					DatastoreRef: corev1.LocalObjectReference{
						Name: datastoreName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())
		})

		It("should wait for datastore to be ready", func() {
			By("Reconciling the User")
			controllerReconciler := &UserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status shows waiting")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, user)
				return err == nil && user.Status.Phase == "Waiting"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When creating a User with missing datastore", func() {
		BeforeEach(func() {
			By("Creating a User resource with non-existent datastore")
			user = &dbv1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name:      userName,
					Namespace: namespace,
				},
				Spec: dbv1.UserSpec{
					Username: "app_user",
					Password: base64.StdEncoding.EncodeToString([]byte("password123")),
					DatastoreRef: corev1.LocalObjectReference{
						Name: "non-existent-datastore",
					},
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())
		})

		It("should handle missing datastore gracefully", func() {
			By("Reconciling the User")
			controllerReconciler := &UserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status shows failure")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, user)
				return err == nil && user.Status.Phase == "Failed"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When testing different database types", func() {
		BeforeEach(func() {
			By("Creating a secret for MySQL")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"username": []byte("testuser"),
					"password": []byte("testpass"),
					"host":     []byte("localhost"),
					"port":     []byte("3306"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating a ready MySQL Datastore")
			datastore = &dbv1.Datastore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      datastoreName,
					Namespace: namespace,
				},
				Spec: dbv1.DatastoreSpec{
					DatastoreType: "mysql",
					SecretRef: dbv1.DatastoreSecretRef{
						Name: secretName,
					},
				},
				Status: dbv1.DatastoreStatus{
					Phase: "Ready",
					Ready: true,
				},
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())
		})

		It("should handle MySQL user creation", func() {
			By("Creating a MySQL User")
			user = &dbv1.User{
				ObjectMeta: metav1.ObjectMeta{
					Name:      userName,
					Namespace: namespace,
				},
				Spec: dbv1.UserSpec{
					Username: "mysql_user",
					Password: base64.StdEncoding.EncodeToString([]byte("password123")),
					DatastoreRef: corev1.LocalObjectReference{
						Name: datastoreName,
					},
					Host: "localhost",
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			By("Reconciling the User")
			controllerReconciler := &UserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, user)
				return err == nil && user.Status.Phase == "Creating"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When handling non-existent resources", func() {
		It("should return without error for non-existent user", func() {
			By("Reconciling a non-existent user")
			controllerReconciler := &UserReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			nonExistentName := types.NamespacedName{
				Name:      "non-existent",
				Namespace: namespace,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: nonExistentName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When testing password encoding", func() {
		It("should handle base64 encoded passwords correctly", func() {
			By("Testing password encoding")
			password := "testpassword123"
			encoded := base64.StdEncoding.EncodeToString([]byte(password))
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(decoded)).To(Equal(password))
		})
	})
})
