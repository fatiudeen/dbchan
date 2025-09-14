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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbv1 "github.com/fatiudeen/dbchan/api/v1"
)

var _ = Describe("Datastore Controller", func() {
	var (
		ctx                context.Context
		datastore          *dbv1.Datastore
		secret             *corev1.Secret
		datastoreName      string
		secretName         string
		namespace          string
		typeNamespacedName types.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
		datastoreName = "test-datastore"
		secretName = "test-secret"
		typeNamespacedName = types.NamespacedName{
			Name:      datastoreName,
			Namespace: namespace,
		}
	})

	AfterEach(func() {
		// Cleanup datastore
		if datastore != nil {
			Expect(k8sClient.Delete(ctx, datastore)).To(Succeed())
		}
		// Cleanup secret
		if secret != nil {
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
		}
	})

	Context("When creating a Datastore with valid secret", func() {
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

			By("Creating a Datastore resource")
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
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())
		})

		It("should update status to Connecting initially", func() {
			By("Reconciling the Datastore")
			controllerReconciler := &DatastoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, datastore)
				return err == nil && datastore.Status.Phase == "Connecting"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})

		It("should handle missing secret gracefully", func() {
			By("Deleting the secret")
			Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

			By("Reconciling the Datastore")
			controllerReconciler := &DatastoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status shows failure")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, datastore)
				return err == nil && datastore.Status.Phase == "Failed"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When creating a Datastore with custom secret keys", func() {
		BeforeEach(func() {
			By("Creating a secret with custom key names")
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"db_user":     []byte("testuser"),
					"db_password": []byte("testpass"),
					"db_host":     []byte("localhost"),
					"db_port":     []byte("5432"),
					"db_ssl":      []byte("disable"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			By("Creating a Datastore resource with custom keys")
			datastore = &dbv1.Datastore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      datastoreName,
					Namespace: namespace,
				},
				Spec: dbv1.DatastoreSpec{
					DatastoreType: "postgres",
					SecretRef: dbv1.DatastoreSecretRef{
						Name:        secretName,
						UsernameKey: "db_user",
						PasswordKey: "db_password",
						HostKey:     "db_host",
						PortKey:     "db_port",
						SSLModeKey:  "db_ssl",
					},
				},
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())
		})

		It("should use custom secret keys", func() {
			By("Reconciling the Datastore")
			controllerReconciler := &DatastoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, datastore)
				return err == nil && datastore.Status.Phase == "Connecting"
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
		})

		It("should handle MySQL datastore type", func() {
			By("Creating a MySQL Datastore")
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
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())

			By("Reconciling the Datastore")
			controllerReconciler := &DatastoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, datastore)
				return err == nil && datastore.Status.Phase == "Connecting"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})

		It("should handle SQL Server datastore type", func() {
			By("Creating a SQL Server Datastore")
			datastore = &dbv1.Datastore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      datastoreName,
					Namespace: namespace,
				},
				Spec: dbv1.DatastoreSpec{
					DatastoreType: "sqlserver",
					SecretRef: dbv1.DatastoreSecretRef{
						Name: secretName,
					},
				},
			}
			Expect(k8sClient.Create(ctx, datastore)).To(Succeed())

			By("Reconciling the Datastore")
			controllerReconciler := &DatastoreReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the status is updated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, datastore)
				return err == nil && datastore.Status.Phase == "Connecting"
			}, time.Second*10, time.Millisecond*100).Should(BeTrue())
		})
	})

	Context("When handling non-existent resources", func() {
		It("should return without error for non-existent datastore", func() {
			By("Reconciling a non-existent datastore")
			controllerReconciler := &DatastoreReconciler{
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

	Context("When testing connection string building", func() {
		It("should build correct connection strings for different database types", func() {
			secretData := map[string][]byte{
				"username": []byte("testuser"),
				"password": []byte("testpass"),
				"host":     []byte("localhost"),
				"port":     []byte("5432"),
				"sslmode":  []byte("disable"),
			}

			secretRef := dbv1.DatastoreSecretRef{
				Name: "test-secret",
			}

			By("Testing PostgreSQL connection string")
			connStr := buildConnectionString("postgres", "testdb", "testuser", secretRef, secretData)
			Expect(connStr).To(ContainSubstring("postgres://testuser:testpass@localhost:5432/testdb"))

			By("Testing MySQL connection string")
			secretData["port"] = []byte("3306")
			connStr = buildConnectionString("mysql", "testdb", "testuser", secretRef, secretData)
			Expect(connStr).To(ContainSubstring("testuser:testpass@tcp(localhost:3306)/testdb"))

			By("Testing SQL Server connection string")
			secretData["port"] = []byte("1433")
			secretData["instance"] = []byte("SQLEXPRESS")
			secretRef.InstanceKey = "instance"
			connStr = buildConnectionString("sqlserver", "testdb", "testuser", secretRef, secretData)
			Expect(connStr).To(ContainSubstring("sqlserver://testuser:testpass@localhost:1433SQLEXPRESS?database=testdb"))
		})
	})
})
